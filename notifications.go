// Package notifications is togo's Laravel-style notification system. A
// Notification is delivered over one or more channels — mail (via togo mail),
// broadcast (via togo realtime), database — and push providers (FCM, OneSignal,
// Pusher) register additional channels via RegisterChannel.
//
// Install: `togo install togo-framework/notifications`.
package notifications

import (
	"context"
	"errors"
	"sync"

	"github.com/togo-framework/mail"
	"github.com/togo-framework/togo"
)

// Notifiable is the recipient and its per-channel routing.
type Notifiable interface {
	RouteID() string         // stable id (database channel)
	RouteEmail() string      // mail channel
	RoutePushTokens() []string // push channels
}

// Notification chooses its channels for a given recipient.
type Notification interface {
	Via(Notifiable) []string
}

// Optional content interfaces a Notification implements per channel:
type (
	// MailNotification renders the email body.
	MailNotification interface{ ToMail(Notifiable) mail.Message }
	// BroadcastNotification renders a realtime event + payload.
	BroadcastNotification interface{ ToBroadcast(Notifiable) (event string, data any) }
	// DatabaseNotification renders the stored payload.
	DatabaseNotification interface{ ToDatabase(Notifiable) map[string]any }
	// PushNotification renders a push message (used by FCM/OneSignal/Pusher).
	PushNotification interface{ ToPush(Notifiable) PushMessage }
)

// PushMessage is the common push payload for push-channel plugins.
type PushMessage struct {
	Title string
	Body  string
	Data  map[string]string
}

// Channel delivers a notification to a recipient.
type Channel interface {
	Send(ctx context.Context, to Notifiable, n Notification) error
}

var (
	chMu             sync.RWMutex
	channelFactories = map[string]func(*togo.Kernel) Channel{}
)

// RegisterChannel registers a delivery channel (call from a plugin's init()).
func RegisterChannel(name string, f func(*togo.Kernel) Channel) {
	chMu.Lock()
	channelFactories[name] = f
	chMu.Unlock()
}

func init() {
	RegisterChannel("mail", func(k *togo.Kernel) Channel { return &mailChannel{k: k} })
	RegisterChannel("broadcast", func(k *togo.Kernel) Channel { return &broadcastChannel{k: k} })
	RegisterChannel("database", func(k *togo.Kernel) Channel { return &dbChannel{k: k} })

	togo.RegisterProviderFunc("notifications", togo.PriorityLate, func(k *togo.Kernel) error {
		svc := &Service{k: k, channels: map[string]Channel{}}
		chMu.RLock()
		for name, f := range channelFactories {
			svc.channels[name] = f(k)
		}
		chMu.RUnlock()
		ensureTable(k)
		k.Set("notifications", svc)
		return nil
	})
}

// Service dispatches notifications to channels.
type Service struct {
	k        *togo.Kernel
	channels map[string]Channel
}

// Send delivers n to every channel returned by n.Via(to).
func (s *Service) Send(ctx context.Context, to Notifiable, n Notification) error {
	var errs []error
	for _, name := range n.Via(to) {
		c, ok := s.channels[name]
		if !ok {
			errs = append(errs, errors.New("notifications: no channel "+name))
			continue
		}
		if err := c.Send(ctx, to, n); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// FromKernel fetches the notifications service from the kernel container.
func FromKernel(k *togo.Kernel) (*Service, bool) {
	v, ok := k.Get("notifications")
	if !ok {
		return nil, false
	}
	s, ok := v.(*Service)
	return s, ok
}
