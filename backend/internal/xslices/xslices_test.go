package xslices

import (
	"encoding/json"
	"strconv"
	"testing"
)

func TestMapConvertsEveryItemInOrder(t *testing.T) {
	got := Map([]int{1, 2, 3}, strconv.Itoa)
	if len(got) != 3 || got[0] != "1" || got[2] != "3" {
		t.Fatalf("got %v", got)
	}
}

func TestMapOfNothingIsAnEmptyListNotNull(t *testing.T) {
	encoded, err := json.Marshal(Map(nil, strconv.Itoa))
	if err != nil || string(encoded) != "[]" {
		t.Fatalf("encoded %s, %v", encoded, err)
	}
}
