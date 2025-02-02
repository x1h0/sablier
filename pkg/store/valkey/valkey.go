package valkey

import (
	"context"
	"encoding/json"
	"github.com/sablierapp/sablier/app/instance"
	"github.com/sablierapp/sablier/pkg/store"
	"github.com/valkey-io/valkey-go"
	"log/slog"
	"strings"
	"time"
)

var _ store.Store = (*ValKey)(nil)

type ValKey struct {
	Client valkey.Client
}

func New(ctx context.Context, client valkey.Client) (store.Store, error) {
	err := client.Do(ctx, client.B().Ping().Build()).Error()
	if err != nil {
		return nil, err
	}

	err = client.Do(ctx, client.B().ConfigSet().ParameterValue().
		ParameterValue("notify-keyspace-events", "KEx").
		Build()).Error()
	if err != nil {
		return nil, err
	}

	return &ValKey{Client: client}, nil
}

func (v *ValKey) Get(ctx context.Context, s string) (instance.State, error) {
	b, err := v.Client.Do(ctx, v.Client.B().Get().Key(s).Build()).AsBytes()
	if valkey.IsValkeyNil(err) {
		return instance.State{}, store.ErrKeyNotFound
	}
	if err != nil {
		return instance.State{}, err
	}

	var i instance.State
	err = json.Unmarshal(b, &i)
	if err != nil {
		return instance.State{}, err
	}

	return i, nil
}

func (v *ValKey) Put(ctx context.Context, state instance.State, duration time.Duration) error {
	value, err := json.Marshal(state)
	if err != nil {
		return err
	}

	return v.Client.Do(ctx, v.Client.B().Set().Key(state.Name).Value(string(value)).Ex(duration).Build()).Error()
}

func (v *ValKey) Delete(ctx context.Context, s string) error {
	return v.Client.Do(ctx, v.Client.B().Del().Key(s).Build()).Error()
}

func (v *ValKey) OnExpire(ctx context.Context, f func(string)) error {
	go func() {
		err := v.Client.Receive(ctx, v.Client.B().Psubscribe().Pattern("__key*__:*").Build(), func(msg valkey.PubSubMessage) {
			if msg.Message == "expired" {
				split := strings.Split(msg.Channel, ":")
				key := split[len(split)-1]
				f(key)
			}
		})
		if err != nil {
			slog.Error("error subscribing", slog.Any("error", err))
		}
	}()
	return nil
}
