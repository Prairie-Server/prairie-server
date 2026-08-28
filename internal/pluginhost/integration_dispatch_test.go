//go:build integration

package pluginhost_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"google.golang.org/protobuf/encoding/protojson"

	pluginv1 "github.com/prairie-server/prairie-plugin-sdk/pkg/pluginproto/prairie/plugin/v1"
	publicmanifest "github.com/prairie-server/prairie-plugin-sdk/pkg/pluginsdk/manifest"

	"github.com/prairie-server/prairie-server/internal/cache"
	"github.com/prairie-server/prairie-server/internal/events"
	"github.com/prairie-server/prairie-server/internal/pluginhost"
	"github.com/prairie-server/prairie-server/internal/plugins"
)

const sdkExamplesDir = "/opt/worktrees/prairie-plugin-sdk-rh/examples"

// fakePlugin captures the host-side state we need per started plugin to wire
// the dispatcher.
type fakePlugin struct {
	installationID int
	pluginID       string
	binaryPath     string
	manifest       *pluginv1.PluginManifest
	parsedManifest *publicmanifest.Manifest
	client         *pluginhost.Client
}

// dispatchTestStore satisfies the dispatcher's internal taskInstallationStore
// interface by name structurally — both methods reference only exported types
// (plugins.Installation, plugins.Capability).
type dispatchTestStore struct {
	installations []*plugins.Installation
	capabilities  map[int][]*plugins.Capability
}

func (s *dispatchTestStore) ListEnabled(_ context.Context) ([]*plugins.Installation, error) {
	return s.installations, nil
}

func (s *dispatchTestStore) ListCapabilities(_ context.Context, id int) ([]*plugins.Capability, error) {
	return s.capabilities[id], nil
}

// dispatchTestResolver maps installation IDs back to their host-side
// pluginhost.Client so the dispatcher can dispense an EventConsumer.
type dispatchTestResolver struct {
	clients map[int]*pluginhost.Client
}

func (r *dispatchTestResolver) EventConsumerClient(_ context.Context, installationID int, capabilityID string) (*pluginhost.EventConsumerClient, error) {
	c, ok := r.clients[installationID]
	if !ok {
		return nil, fmt.Errorf("no client for installation %d", installationID)
	}
	return c.EventConsumer(capabilityID)
}

// inMemoryBus is a minimal cache.EventBus for tests. The dispatcher subscribes
// to it but for this integration test we drive events through the hub directly
// (plugin->host RuntimeHost.PublishEvent feeds the hub), so the bus never sees
// traffic. A working impl is still required for dispatcher.Start.
type inMemoryBus struct {
	mu       sync.Mutex
	handlers map[string][]cache.EventHandler
}

func newInMemoryBus() *inMemoryBus { return &inMemoryBus{handlers: map[string][]cache.EventHandler{}} }

func (b *inMemoryBus) Publish(_ context.Context, channel string, ev cache.Event) error {
	b.mu.Lock()
	hs := append([]cache.EventHandler(nil), b.handlers[channel]...)
	b.mu.Unlock()
	for _, h := range hs {
		h(ev)
	}
	return nil
}

func (b *inMemoryBus) Subscribe(_ context.Context, channel string, h cache.EventHandler) error {
	b.mu.Lock()
	b.handlers[channel] = append(b.handlers[channel], h)
	b.mu.Unlock()
	return nil
}

func (b *inMemoryBus) Close() error { return nil }

// installationViewFromParsed turns a parsed SDK manifest into the dispatcher's
// InstallationView, copying only the event_consumer.v1 capability's
// subscriptions_by_capability data the dispatcher needs.
func installationViewFromParsed(installationID int, m *publicmanifest.Manifest) plugins.InstallationView {
	view := plugins.InstallationView{
		InstallationID: installationID,
		PluginID:       m.GetPluginId(),
	}
	for _, cap := range m.Capabilities {
		for _, sub := range cap.SubscriptionsByCapability {
			view.SubscriptionsByCapability = append(view.SubscriptionsByCapability, plugins.CapabilitySubscription{
				Capability: sub.Capability,
				Events:     append([]string(nil), sub.Events...),
			})
		}
	}
	return view
}

// capabilitiesFromManifest mirrors the production conversion: it asks
// plugins.CapabilityRecordsFromManifest for the host-side records (which
// flatten the proto's Subscriptions/DisplayName/etc into Metadata["..."]),
// then stamps InstallationID on each.
func capabilitiesFromManifest(t *testing.T, installationID int, manifest *pluginv1.PluginManifest) []*plugins.Capability {
	t.Helper()
	records, err := plugins.CapabilityRecordsFromManifest(manifest)
	if err != nil {
		t.Fatalf("CapabilityRecordsFromManifest: %v", err)
	}
	out := make([]*plugins.Capability, 0, len(records))
	for i := range records {
		records[i].InstallationID = installationID
		out = append(out, &records[i])
	}
	return out
}

func buildSDKExample(t *testing.T, name string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = filepath.Join(sdkExamplesDir, name)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", name, err, b)
	}
	return out
}

// startFakePlugin builds the named SDK example, reads its proto manifest from
// the binary's `manifest` subcommand and its on-disk manifest.json (which
// carries the proto-extension fields the proto-only manifest lacks), starts
// the plugin via host.Start, and returns the captured state.
func startFakePlugin(t *testing.T, host *pluginhost.Host, name string, installationID int) *fakePlugin {
	t.Helper()
	bin := buildSDKExample(t, name)

	manifestBytes, err := exec.Command(bin, "manifest").Output()
	if err != nil {
		t.Fatalf("get manifest from %s: %v", name, err)
	}
	manifest := &pluginv1.PluginManifest{}
	if err := protojson.Unmarshal(manifestBytes, manifest); err != nil {
		t.Fatalf("parse manifest protojson for %s: %v", name, err)
	}

	srcManifest, err := os.ReadFile(filepath.Join(sdkExamplesDir, name, "manifest.json"))
	if err != nil {
		t.Fatalf("read source manifest for %s: %v", name, err)
	}
	parsed, err := publicmanifest.Parse(srcManifest)
	if err != nil {
		t.Fatalf("publicmanifest.Parse(%s): %v", name, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := host.Start(ctx, pluginhost.StartRequest{
		InstallationID: installationID,
		BinaryPath:     bin,
		Manifest:       manifest,
	})
	if err != nil {
		t.Fatalf("host.Start(%s): %v", name, err)
	}
	t.Cleanup(func() { _ = host.Stop(installationID) })

	return &fakePlugin{
		installationID: installationID,
		pluginID:       manifest.GetPluginId(),
		binaryPath:     bin,
		manifest:       manifest,
		parsedManifest: parsed,
		client:         client,
	}
}

// triggerScheduledTask fires the given scheduled-task capability on the plugin.
func triggerScheduledTask(t *testing.T, p *fakePlugin, capabilityID string) {
	t.Helper()
	taskClient, err := p.client.ScheduledTask(capabilityID)
	if err != nil {
		t.Fatalf("ScheduledTask(%s) on %s: %v", capabilityID, p.pluginID, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := taskClient.Run(ctx, &pluginv1.RunScheduledTaskRequest{TaskKey: capabilityID}); err != nil {
		t.Fatalf("Run(%s) on %s: %v", capabilityID, p.pluginID, err)
	}
}

// hubObserver subscribes to a hub and records every envelope it sees.
type hubObserver struct {
	mu     sync.Mutex
	events []events.Envelope
}

func newHubObserver(t *testing.T, hub *events.Hub) *hubObserver {
	t.Helper()
	o := &hubObserver{}
	ch, unsub := hub.Subscribe()
	t.Cleanup(unsub)
	go func() {
		for env := range ch {
			o.mu.Lock()
			o.events = append(o.events, env)
			o.mu.Unlock()
		}
	}()
	return o
}

func (o *hubObserver) seen() []events.Envelope {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]events.Envelope, len(o.events))
	copy(out, o.events)
	return out
}

func (o *hubObserver) waitFor(t *testing.T, predicate func(events.Envelope) bool, timeout time.Duration) (events.Envelope, bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, env := range o.seen() {
			if predicate(env) {
				return env, true
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return events.Envelope{}, false
}

// TestCapabilityScopedDispatchEndToEnd asserts that a consumer subscribed to
// `request_router.v1` capability via `subscriptions_by_capability` receives an
// event published by a router that declares `request_router.v1`.
func TestCapabilityScopedDispatchEndToEnd(t *testing.T) {
	bus := newInMemoryBus()
	hub := events.NewHub("test", bus)
	if err := hub.Start(context.Background()); err != nil {
		t.Fatalf("hub.Start: %v", err)
	}

	observer := newHubObserver(t, hub)

	host := pluginhost.NewHost(pluginhost.Config{
		Logger:         hclog.NewNullLogger(),
		EventPublisher: hub,
	})

	router := startFakePlugin(t, host, "fake-router", 1)
	consumer := startFakePlugin(t, host, "fake-consumer", 2)

	store := &dispatchTestStore{
		installations: []*plugins.Installation{
			{ID: router.installationID, PluginID: router.pluginID, Enabled: true},
			{ID: consumer.installationID, PluginID: consumer.pluginID, Enabled: true},
		},
		capabilities: map[int][]*plugins.Capability{
			router.installationID:   capabilitiesFromManifest(t, router.installationID, router.manifest),
			consumer.installationID: capabilitiesFromManifest(t, consumer.installationID, consumer.manifest),
		},
	}
	resolver := &dispatchTestResolver{clients: map[int]*pluginhost.Client{
		router.installationID:   router.client,
		consumer.installationID: consumer.client,
	}}

	dispatcher := plugins.NewEventDispatcherWithTypedResolver(bus, hub, store, resolver, 4)
	if err := dispatcher.Start(context.Background()); err != nil {
		t.Fatalf("dispatcher.Start: %v", err)
	}
	t.Cleanup(dispatcher.Stop)

	dispatcher.OnInstallationsChanged([]plugins.InstallationView{
		installationViewFromParsed(router.installationID, router.parsedManifest),
		installationViewFromParsed(consumer.installationID, consumer.parsedManifest),
	})

	triggerScheduledTask(t, router, "emit-once")

	emitted := "plugin." + router.pluginID + ".submitted"
	forwarded := "plugin." + consumer.pluginID + ".received." + emitted

	if _, ok := observer.waitFor(t, func(env events.Envelope) bool { return env.Event == emitted }, 5*time.Second); !ok {
		t.Fatalf("router emission %q never reached the hub. seen: %v", emitted, eventNames(observer.seen()))
	}
	if _, ok := observer.waitFor(t, func(env events.Envelope) bool { return env.Event == forwarded }, 5*time.Second); !ok {
		t.Fatalf("consumer never re-published %q (capability-scoped dispatch failed). seen: %v", forwarded, eventNames(observer.seen()))
	}
}

// TestTargetedPublishReachesOnlyNamedPlugin asserts that when a publisher uses
// PublishEventTo, only the named target plugin's HandleEvent is invoked even
// though another consumer subscribes to the same event by name.
func TestTargetedPublishReachesOnlyNamedPlugin(t *testing.T) {
	bus := newInMemoryBus()
	hub := events.NewHub("test", bus)
	if err := hub.Start(context.Background()); err != nil {
		t.Fatalf("hub.Start: %v", err)
	}

	observer := newHubObserver(t, hub)

	host := pluginhost.NewHost(pluginhost.Config{
		Logger:         hclog.NewNullLogger(),
		EventPublisher: hub,
	})

	publisher := startFakePlugin(t, host, "fake-publisher", 10)
	consumer := startFakePlugin(t, host, "fake-consumer", 11) // also subscribes to foo-event by name; should NOT receive
	target := startFakePlugin(t, host, "fake-target", 12)     // subscribes to foo-event by name; SHOULD receive

	store := &dispatchTestStore{
		installations: []*plugins.Installation{
			{ID: publisher.installationID, PluginID: publisher.pluginID, Enabled: true},
			{ID: consumer.installationID, PluginID: consumer.pluginID, Enabled: true},
			{ID: target.installationID, PluginID: target.pluginID, Enabled: true},
		},
		capabilities: map[int][]*plugins.Capability{
			publisher.installationID: capabilitiesFromManifest(t, publisher.installationID, publisher.manifest),
			consumer.installationID:  capabilitiesFromManifest(t, consumer.installationID, consumer.manifest),
			target.installationID:    capabilitiesFromManifest(t, target.installationID, target.manifest),
		},
	}
	resolver := &dispatchTestResolver{clients: map[int]*pluginhost.Client{
		publisher.installationID: publisher.client,
		consumer.installationID:  consumer.client,
		target.installationID:    target.client,
	}}

	dispatcher := plugins.NewEventDispatcherWithTypedResolver(bus, hub, store, resolver, 4)
	if err := dispatcher.Start(context.Background()); err != nil {
		t.Fatalf("dispatcher.Start: %v", err)
	}
	t.Cleanup(dispatcher.Stop)

	dispatcher.OnInstallationsChanged([]plugins.InstallationView{
		installationViewFromParsed(publisher.installationID, publisher.parsedManifest),
		installationViewFromParsed(consumer.installationID, consumer.parsedManifest),
		installationViewFromParsed(target.installationID, target.parsedManifest),
	})

	triggerScheduledTask(t, publisher, "emit-targeted")

	emitted := "plugin." + publisher.pluginID + ".foo-event"
	targetForward := "plugin." + target.pluginID + ".received." + emitted
	consumerForward := "plugin." + consumer.pluginID + ".received." + emitted

	if _, ok := observer.waitFor(t, func(env events.Envelope) bool { return env.Event == emitted }, 5*time.Second); !ok {
		t.Fatalf("publisher emission %q never reached the hub. seen: %v", emitted, eventNames(observer.seen()))
	}
	if _, ok := observer.waitFor(t, func(env events.Envelope) bool { return env.Event == targetForward }, 5*time.Second); !ok {
		t.Fatalf("target never re-published %q (targeted dispatch failed). seen: %v", targetForward, eventNames(observer.seen()))
	}

	// Now confirm the non-target consumer remained silent. Wait a bit past the
	// point where target already forwarded — if consumer were going to receive,
	// it would have done so by now.
	if _, ok := observer.waitFor(t, func(env events.Envelope) bool { return env.Event == consumerForward }, 750*time.Millisecond); ok {
		t.Fatalf("non-target consumer received %q — target filter failed. seen: %v", consumerForward, eventNames(observer.seen()))
	}
}

func eventNames(envs []events.Envelope) string {
	names := make([]string, 0, len(envs))
	for _, e := range envs {
		names = append(names, e.Event)
	}
	return strings.Join(names, ", ")
}
