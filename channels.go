package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/togo-framework/mail"
	"github.com/togo-framework/togo"
)

// mailChannel sends via the togo mail plugin (if installed).
type mailChannel struct{ k *togo.Kernel }

func (c *mailChannel) Send(ctx context.Context, to Notifiable, n Notification) error {
	mn, ok := n.(MailNotification)
	if !ok {
		return nil
	}
	svc, ok := mail.FromKernel(c.k)
	if !ok {
		return errors.New("notifications: mail plugin not installed")
	}
	msg := mn.ToMail(to)
	if len(msg.To) == 0 {
		msg.To = []string{to.RouteEmail()}
	}
	return svc.Send(ctx, msg)
}

// broadcastChannel pushes a realtime event via the togo realtime broker.
type broadcastChannel struct{ k *togo.Kernel }

func (c *broadcastChannel) Send(_ context.Context, to Notifiable, n Notification) error {
	bn, ok := n.(BroadcastNotification)
	if !ok {
		return nil
	}
	if c.k.Realtime == nil {
		return errors.New("notifications: realtime plugin not installed")
	}
	event, data := bn.ToBroadcast(to)
	payload, err := json.Marshal(data)
	if err != nil {
		payload = []byte("{}")
	}
	c.k.Realtime.Publish(event, string(payload))
	return nil
}

// dbChannel stores the notification in the notifications table.
type dbChannel struct{ k *togo.Kernel }

func (c *dbChannel) Send(ctx context.Context, to Notifiable, n Notification) error {
	dn, ok := n.(DatabaseNotification)
	if !ok {
		return nil
	}
	db, err := c.k.SQL(ctx)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(dn.ToDatabase(to))
	d := c.k.Dialect()
	// Only the trusted dialect placeholders ("?"/"$N") are concatenated; all
	// values are bound parameters.
	q := "INSERT INTO notifications (id, notifiable_id, data, created_at) VALUES (" + //#nosec G202 -- dialect placeholders only; values parameterized
		d.Placeholder(1) + ", " + d.Placeholder(2) + ", " + d.Placeholder(3) + ", " + d.Placeholder(4) + ")"
	_, err = db.ExecContext(ctx, q,
		randomID(), to.RouteID(), string(data), time.Now().UTC().Format(time.RFC3339))
	return err
}

// ensureTable creates the notifications table (all TEXT for cross-driver use).
func ensureTable(k *togo.Kernel) {
	db, err := k.SQL(context.Background())
	if err != nil {
		return
	}
	_, _ = db.ExecContext(context.Background(), `CREATE TABLE IF NOT EXISTS notifications (
		id text PRIMARY KEY,
		notifiable_id text NOT NULL,
		data text NOT NULL,
		read_at text,
		created_at text NOT NULL
	)`)
}
