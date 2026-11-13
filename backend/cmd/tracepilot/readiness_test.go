package main

import (
	"context"
	"errors"
	"testing"
)

type fakeComponent struct{ ready bool }

func (component fakeComponent) Ready() bool { return component.ready }

type fakeDatabase struct{ err error }

func (database fakeDatabase) Ready(context.Context) error { return database.err }

func TestReadinessRequiresEveryComponent(t *testing.T) {
	events, metrics, listener := fakeComponent{true}, fakeComponent{true}, fakeComponent{true}
	if err := buildReadinessCheck(fakeDatabase{}, events, metrics, listener)(context.Background()); err != nil {
		t.Fatalf("all components ready: %v", err)
	}
	for name, parts := range map[string][]readyReporter{
		"event pipeline":   {fakeComponent{false}, metrics, listener},
		"metrics pipeline": {events, fakeComponent{false}, listener},
		"OTLP listener":    {events, metrics, fakeComponent{false}},
	} {
		if err := buildReadinessCheck(fakeDatabase{}, parts...)(context.Background()); err == nil {
			t.Errorf("%s was down but the service reported ready", name)
		}
	}
	failing := errors.New("schema is dirty")
	if err := buildReadinessCheck(fakeDatabase{err: failing}, events, metrics, listener)(context.Background()); !errors.Is(err, failing) {
		t.Fatalf("a database failure must surface, got %v", err)
	}
}
