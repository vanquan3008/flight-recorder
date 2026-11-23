package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeComponent struct{ ready bool }

func (component fakeComponent) Ready() bool { return component.ready }

type fakeDatabase struct{ err error }

func (database fakeDatabase) Ready(context.Context) error { return database.err }

func TestReadinessRequiresEveryComponent(t *testing.T) {
	named := func(events, metrics, listener bool) []namedReadyComponent {
		return []namedReadyComponent{
			{name: "event pipeline", reporter: fakeComponent{events}},
			{name: "metrics pipeline", reporter: fakeComponent{metrics}},
			{name: "OTLP listener", reporter: fakeComponent{listener}},
		}
	}
	if err := buildReadinessCheck(fakeDatabase{}, named(true, true, true)...)(context.Background()); err != nil {
		t.Fatalf("all components ready: %v", err)
	}
	for name, parts := range map[string][]namedReadyComponent{
		"event pipeline":   named(false, true, true),
		"metrics pipeline": named(true, false, true),
		"OTLP listener":    named(true, true, false),
	} {
		err := buildReadinessCheck(fakeDatabase{}, parts...)(context.Background())
		if err == nil {
			t.Errorf("%s was down but the service reported ready", name)
		} else if !strings.Contains(err.Error(), name) {
			t.Errorf("readiness error %q does not name %s", err, name)
		}
	}
	missing := []namedReadyComponent{{name: "OTLP listener"}}
	if err := buildReadinessCheck(fakeDatabase{}, missing...)(context.Background()); err == nil {
		t.Error("a component without a reporter was reported ready")
	}
	failing := errors.New("schema is dirty")
	if err := buildReadinessCheck(fakeDatabase{err: failing}, named(true, true, true)...)(context.Background()); !errors.Is(err, failing) {
		t.Fatalf("a database failure must surface, got %v", err)
	}
}
