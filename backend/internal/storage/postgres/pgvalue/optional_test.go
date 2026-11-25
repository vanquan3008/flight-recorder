package pgvalue

import (
	"testing"
	"time"
)

func TestOptionalValuesBecomeNull(t *testing.T) {
	if OrNil("") != nil || OrNil(0) != nil || *OrNil("checkout") != "checkout" {
		t.Fatal("OrNil must map only the zero value to NULL")
	}
	if TimeOrNil(time.Time{}) != nil || TimeOrNil(time.Unix(1, 0)) == nil {
		t.Fatal("TimeOrNil must map only the zero time to NULL")
	}
	if NanosOrNil(0) != nil || *NanosOrNil(time.Millisecond) != 1_000_000 {
		t.Fatal("NanosOrNil must store nanoseconds")
	}
}

func TestNumericConversionsStayInRange(t *testing.T) {
	status := int32(503)
	if IntPointer(nil) != nil || *IntPointer(&status) != 503 {
		t.Fatal("IntPointer must widen without losing NULL")
	}
}

func TestStringOrEmptyReadsNullAsEmpty(t *testing.T) {
	value := "checkout"
	if StringOrEmpty(nil) != "" || StringOrEmpty(&value) != "checkout" {
		t.Fatal("StringOrEmpty must map only NULL to the empty string")
	}
}
