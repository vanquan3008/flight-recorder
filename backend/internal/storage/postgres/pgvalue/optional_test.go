package pgvalue

import (
	"math"
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
	if BytesOrNil([]byte{}) != nil || BytesOrNil([]byte("[]")) == nil {
		t.Fatal("BytesOrNil must map only empty documents to NULL")
	}
}

func TestNumericConversionsStayInRange(t *testing.T) {
	if ClampInt32(math.MaxUint32) != math.MaxInt32 || ClampInt32(7) != 7 {
		t.Fatal("ClampInt32 must saturate instead of wrapping negative")
	}
	nan, infinite, finite := math.NaN(), math.Inf(1), 1.5
	if FiniteOrNil(&nan) != nil || FiniteOrNil(&infinite) != nil || FiniteOrNil(nil) != nil || *FiniteOrNil(&finite) != 1.5 {
		t.Fatal("FiniteOrNil must store only finite values")
	}
	status := int32(503)
	if IntPointer(nil) != nil || *IntPointer(&status) != 503 {
		t.Fatal("IntPointer must widen without losing NULL")
	}
}
