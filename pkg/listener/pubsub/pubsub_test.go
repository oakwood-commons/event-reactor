package pubsub

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"cloud.google.com/go/pubsub/v2/pstest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/oakwood-commons/event-reactor/pkg/message"
)

const (
	testProject = "test-project"
	testTopic   = "projects/test-project/topics/events"
	testSubID   = "sub"
	testSubName = "projects/test-project/subscriptions/sub"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// setupFake starts an in-process Pub/Sub fake, creates the topic and
// subscription, and returns the server plus client options that point a client
// at the fake.
func setupFake(t *testing.T) (*pstest.Server, []option.ClientOption) {
	t.Helper()

	srv := pstest.NewServer()
	t.Cleanup(func() { _ = srv.Close() })

	conn, err := grpc.NewClient(srv.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	opts := []option.ClientOption{option.WithGRPCConn(conn)}

	admin, err := pubsub.NewClient(context.Background(), testProject, opts...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = admin.Close() })

	_, err = admin.TopicAdminClient.CreateTopic(context.Background(), &pubsubpb.Topic{Name: testTopic})
	require.NoError(t, err)
	_, err = admin.SubscriptionAdminClient.CreateSubscription(context.Background(), &pubsubpb.Subscription{
		Name:  testSubName,
		Topic: testTopic,
	})
	require.NoError(t, err)

	return srv, opts
}

// newTestListener builds a listener wired to the fake server.
func newTestListener(t *testing.T, opts []option.ClientOption) *Listener {
	t.Helper()
	l, err := New(Config{
		Name:           "test-pubsub",
		ProjectID:      testProject,
		SubscriptionID: testSubID,
	}, testLogger())
	require.NoError(t, err)
	l.clientOpts = opts
	return l
}

// waitForAcks polls the fake until the message has been acked the expected
// number of times, or fails after a timeout.
func waitForAcks(t *testing.T, srv *pstest.Server, id string, want int) {
	t.Helper()
	require.Eventually(t, func() bool {
		m := srv.Message(id)
		return m != nil && m.Acks >= want
	}, 3*time.Second, 10*time.Millisecond, "message %s was not acked %d time(s)", id, want)
}

func TestNew_Validation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name:    "missing projectId",
			cfg:     Config{Name: "l", SubscriptionID: "s"},
			wantErr: "projectId is required",
		},
		{
			name:    "missing subscriptionId",
			cfg:     Config{Name: "l", ProjectID: "p"},
			wantErr: "subscriptionId is required",
		},
		{
			name: "valid",
			cfg:  Config{Name: "l", ProjectID: "p", SubscriptionID: "s"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l, err := New(tc.cfg, testLogger())
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "l", l.Name())
		})
	}
}

func TestStart_DeliversAndAcks(t *testing.T) {
	srv, opts := setupFake(t)
	l := newTestListener(t, opts)

	received := make(chan message.Event, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- l.Start(ctx, func(_ context.Context, e message.Event) {
			received <- e
		})
	}()

	id := srv.Publish(testTopic, []byte(`{"data":{"new":{"billingID":"58655"}}}`), map[string]string{
		"ce-type":      "com.ford.fcp.platform-assets.eamsId.notFound",
		"ce-source":    "https://platform-assets.fcp.ford.com/data-sync",
		"notification": "true",
	})

	select {
	case e := <-received:
		assert.Equal(t, "com.ford.fcp.platform-assets.eamsId.notFound", e.Type)
		assert.Equal(t, "https://platform-assets.fcp.ford.com/data-sync", e.Source)
		assert.Equal(t, "true", e.Attributes["notification"])
		payload, ok := e.Payload.(map[string]any)
		require.True(t, ok)
		data, ok := payload["data"].(map[string]any)
		require.True(t, ok)
		newData, ok := data["new"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "58655", newData["billingID"])
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for delivered event")
	}

	waitForAcks(t, srv, id, 1)

	cancel()
	assert.NoError(t, <-errCh)
}

func TestStart_NacksUnconvertibleMessage(t *testing.T) {
	srv, opts := setupFake(t)
	l := newTestListener(t, opts)

	var mu sync.Mutex
	handlerCalls := 0

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- l.Start(ctx, func(_ context.Context, _ message.Event) {
			mu.Lock()
			handlerCalls++
			mu.Unlock()
		})
	}()

	// Non-JSON, non-base64 data cannot be converted into an event.
	id := srv.Publish(testTopic, []byte("not-json-and-not-base64!!!"), nil)

	// The message must be delivered (attempted) but never acked, and the
	// handler must never see an unconvertible message.
	require.Eventually(t, func() bool {
		m := srv.Message(id)
		return m != nil && m.Deliveries >= 1
	}, 3*time.Second, 10*time.Millisecond, "unconvertible message was never delivered")

	mu.Lock()
	calls := handlerCalls
	mu.Unlock()
	assert.Zero(t, calls, "handler must not be invoked for unconvertible messages")
	assert.Zero(t, srv.Message(id).Acks, "unconvertible message must not be acked")

	cancel()
	assert.NoError(t, <-errCh)
}

func TestStart_NacksOnHandlerPanic(t *testing.T) {
	srv, opts := setupFake(t)
	l := newTestListener(t, opts)

	panicked := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- l.Start(ctx, func(_ context.Context, _ message.Event) {
			select {
			case panicked <- struct{}{}:
			default:
			}
			panic("boom")
		})
	}()

	id := srv.Publish(testTopic, []byte(`{"ok":true}`), nil)

	select {
	case <-panicked:
	case <-time.After(3 * time.Second):
		t.Fatal("handler was never invoked")
	}

	// A panicking handler must not ack; the listener must survive the panic.
	assert.Zero(t, srv.Message(id).Acks, "panicked message must not be acked")

	cancel()
	assert.NoError(t, <-errCh, "listener must shut down cleanly after a handler panic")
}

func TestStart_CleanShutdownOnCancel(t *testing.T) {
	_, opts := setupFake(t)
	l := newTestListener(t, opts)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- l.Start(ctx, func(_ context.Context, _ message.Event) {})
	}()

	// Give Receive a moment to establish the streaming pull, then cancel.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("listener did not shut down after context cancellation")
	}
}
