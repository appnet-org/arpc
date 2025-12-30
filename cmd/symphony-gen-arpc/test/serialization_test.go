package Test

import (
	"bytes"
	"math"
	"reflect"
	"testing"
)

// --- Helpers ---

// SymphonyMessage is the interface for the standard struct implementation
type SymphonyMessage interface {
	MarshalSymphony() ([]byte, error)
	UnmarshalSymphony([]byte) error
}

// runRoundTrip tests the standard struct implementation
func runRoundTrip[T SymphonyMessage](t *testing.T, input T, factory func() T) {
	t.Helper()

	// 1. Marshal
	data, err := input.MarshalSymphony()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// 2. Unmarshal
	output := factory()
	if err := output.UnmarshalSymphony(data); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// 3. Compare
	if !reflect.DeepEqual(input, output) {
		t.Errorf("Mismatch after round trip.\nInput:  %+v\nOutput: %+v", input, output)
	}
}

// --- Tests ---

func TestFixed(t *testing.T) {
	t.Run("Struct_RoundTrip", func(t *testing.T) {
		msg := &Fixed{
			FInt32:  math.MinInt32,
			FInt64:  math.MinInt64,
			FUint32: math.MaxUint32,
			FUint64: math.MaxUint64,
			FBool:   true,
			FFloat:  3.14159,
			FDouble: 1.23456789,
		}
		runRoundTrip(t, msg, func() *Fixed { return &Fixed{} })
	})

	t.Run("Raw_Lifecycle_AllFields", func(t *testing.T) {
		// Note: With public/private segments, only private fields can be mutated on complete buffers
		// Public fields can only be mutated on public-only buffers

		// 1. Create bytes via Struct
		origin := &Fixed{}
		data, _ := origin.MarshalSymphony()

		// 2. Unmarshal into Raw
		var raw FixedRaw
		if err := raw.UnmarshalSymphony(data); err != nil {
			t.Fatalf("UnmarshalSymphony failed: %v", err)
		}

		// 3. Mutate Private Fields Only (on complete buffer)
		// Public fields: FInt32, FUint32, FBool, FDouble
		// Private fields: FInt64, FUint64, FFloat
		if err := raw.SetFInt64(math.MinInt64); err != nil {
			t.Fatal(err)
		}
		if err := raw.SetFUint64(math.MaxUint64); err != nil {
			t.Fatal(err)
		}
		if err := raw.SetFFloat(1.234); err != nil {
			t.Fatal(err)
		}

		// 4. Verify Private Fields
		if got := raw.GetFInt64(); got != math.MinInt64 {
			t.Errorf("FInt64: %v", got)
		}
		if got := raw.GetFUint64(); got != math.MaxUint64 {
			t.Errorf("FUint64: %v", got)
		}
		if got := raw.GetFFloat(); got != 1.234 {
			t.Errorf("FFloat: %v", got)
		}

		// 5. Test public field mutation on public-only buffer
		offsetToPrivate := int(data[1]) | int(data[2])<<8 | int(data[3])<<16 | int(data[4])<<24
		publicOnly := data[:offsetToPrivate]

		// Directly assign public-only buffer (no unmarshal needed for Raw types)
		publicRaw := FixedRaw(publicOnly)

		// Mutate public fields on public-only buffer
		if err := publicRaw.SetFInt32(math.MinInt32); err != nil {
			t.Fatal(err)
		}
		if err := publicRaw.SetFUint32(math.MaxUint32); err != nil {
			t.Fatal(err)
		}
		if err := publicRaw.SetFBool(true); err != nil {
			t.Fatal(err)
		}
		if err := publicRaw.SetFDouble(5.6789); err != nil {
			t.Fatal(err)
		}

		// Verify public fields
		if got := publicRaw.GetFInt32(); got != math.MinInt32 {
			t.Errorf("FInt32: %v", got)
		}
		if got := publicRaw.GetFUint32(); got != math.MaxUint32 {
			t.Errorf("FUint32: %v", got)
		}
		if got := publicRaw.GetFBool(); !got {
			t.Errorf("FBool: %v", got)
		}
		if got := publicRaw.GetFDouble(); got != 5.6789 {
			t.Errorf("FDouble: %v", got)
		}
	})
}

func TestVar(t *testing.T) {
	t.Run("Struct_RoundTrip", func(t *testing.T) {
		msg := &Var{
			VString: "Symphony",
			VBytes:  []byte{0xFF, 0xAA},
		}
		runRoundTrip(t, msg, func() *Var { return &Var{} })
	})

	t.Run("Raw_Mutation_Lifecycle", func(t *testing.T) {
		// VString is public, VBytes is private
		origin := &Var{VString: "init", VBytes: []byte{}}
		completeData, _ := origin.MarshalSymphony()

		// Test private field (VBytes) mutation on complete buffer
		var raw VarRaw
		if err := raw.UnmarshalSymphony(completeData); err != nil {
			t.Fatal(err)
		}

		newBytes := []byte{1, 2, 3, 4}
		if err := raw.SetVBytes(newBytes); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(raw.GetVBytes(), newBytes) {
			t.Error("Bytes mismatch")
		}

		// Test public field (VString) mutation on public-only buffer
		offsetToPrivate := int(completeData[1]) | int(completeData[2])<<8 |
			int(completeData[3])<<16 | int(completeData[4])<<24
		publicOnly := completeData[:offsetToPrivate]

		// Directly assign public-only buffer (no unmarshal needed for Raw types)
		publicRaw := VarRaw(publicOnly)

		if err := publicRaw.SetVString("modified_string"); err != nil {
			t.Fatal(err)
		}
		got := publicRaw.GetVString()
		if got != "modified_string" {
			t.Errorf("String mismatch: got %q, want %q", got, "modified_string")
		}
	})
}

func TestRepeatedFixed(t *testing.T) {
	t.Run("Struct_RoundTrip_FullCoverage", func(t *testing.T) {
		msg := &RepeatedFixed{
			RInt32:  []int32{1, -1, math.MaxInt32},
			RInt64:  []int64{100, -100, math.MaxInt64},
			RUint32: []uint32{0, 100, math.MaxUint32},
			RUint64: []uint64{0, 1000, math.MaxUint64},
			RFloat:  []float32{1.1, 2.2, -3.3},
			RDouble: []float64{10.01, 20.02, -30.03},
			RBool:   []bool{true, false, true},
		}
		runRoundTrip(t, msg, func() *RepeatedFixed { return &RepeatedFixed{} })
	})

	t.Run("Raw_GetSet_AllTypes", func(t *testing.T) {
		// PUBLIC: r_int64, r_uint64, r_double
		// PRIVATE: r_int32, r_uint32, r_float, r_bool

		origin := &RepeatedFixed{}
		completeData, _ := origin.MarshalSymphony()

		// Test private fields on complete buffer
		var raw RepeatedFixedRaw
		if err := raw.UnmarshalSymphony(completeData); err != nil {
			t.Fatal(err)
		}

		// Test Int32 (private)
		vInt32 := []int32{1, 2, 3}
		if err := raw.SetRInt32(vInt32); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(raw.GetRInt32(), vInt32) {
			t.Error("RInt32 mismatch")
		}

		// Test Uint32 (private)
		vUint32 := []uint32{5, 6, 7}
		if err := raw.SetRUint32(vUint32); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(raw.GetRUint32(), vUint32) {
			t.Error("RUint32 mismatch")
		}

		// Test Float (private)
		vFloat := []float32{1.1, 2.2}
		if err := raw.SetRFloat(vFloat); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(raw.GetRFloat(), vFloat) {
			t.Error("RFloat mismatch")
		}

		// Test Bool (private)
		vBool := []bool{true, false, true}
		if err := raw.SetRBool(vBool); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(raw.GetRBool(), vBool) {
			t.Error("RBool mismatch")
		}

		// Test public fields on public-only buffer
		offsetToPrivate := int(completeData[1]) | int(completeData[2])<<8 |
			int(completeData[3])<<16 | int(completeData[4])<<24
		publicOnly := completeData[:offsetToPrivate]

		// Directly assign public-only buffer (no unmarshal needed for Raw types)
		publicRaw := RepeatedFixedRaw(publicOnly)

		// Test Int64 (public)
		vInt64 := []int64{10, 20, 30}
		if err := publicRaw.SetRInt64(vInt64); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(publicRaw.GetRInt64(), vInt64) {
			t.Error("RInt64 mismatch")
		}

		// Test Uint64 (public)
		vUint64 := []uint64{50, 60, 70}
		if err := publicRaw.SetRUint64(vUint64); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(publicRaw.GetRUint64(), vUint64) {
			t.Error("RUint64 mismatch")
		}

		// Test Double (public)
		vDouble := []float64{1.11, 2.22}
		if err := publicRaw.SetRDouble(vDouble); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(publicRaw.GetRDouble(), vDouble) {
			t.Error("RDouble mismatch")
		}
	})
}

func TestRepeatedVar(t *testing.T) {
	t.Run("Struct_RoundTrip", func(t *testing.T) {
		msg := &RepeatedVar{
			RString: []string{"one", "two", ""},
			RBytes:  [][]byte{{1}, {2, 3}, {}},
		}
		runRoundTrip(t, msg, func() *RepeatedVar { return &RepeatedVar{} })
	})

	t.Run("Raw_GetSet", func(t *testing.T) {
		// PUBLIC: r_string
		// PRIVATE: r_bytes

		origin := &RepeatedVar{}
		completeData, _ := origin.MarshalSymphony()

		// Test private field (r_bytes) on complete buffer
		var raw RepeatedVarRaw
		if err := raw.UnmarshalSymphony(completeData); err != nil {
			t.Fatal(err)
		}

		bs := [][]byte{{0x01}, {0x02, 0x03}}
		if err := raw.SetRBytes(bs); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(raw.GetRBytes(), bs) {
			t.Error("RBytes mismatch")
		}

		// Test public field (r_string) on public-only buffer
		offsetToPrivate := int(completeData[1]) | int(completeData[2])<<8 |
			int(completeData[3])<<16 | int(completeData[4])<<24
		publicOnly := completeData[:offsetToPrivate]

		// Directly assign public-only buffer (no unmarshal needed for Raw types)
		publicRaw := RepeatedVarRaw(publicOnly)

		strs := []string{"A", "B", "C"}
		if err := publicRaw.SetRString(strs); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(publicRaw.GetRString(), strs) {
			t.Error("RString mismatch")
		}
	})
}

func TestDeepNested(t *testing.T) {
	t.Run("Struct_RoundTrip", func(t *testing.T) {
		msg := &Root{
			RootId: 1,
			L1: &Level1{
				L1Data: "L1",
				L2: &Level2{
					Leaf: &Leaf{LeafId: 10, LeafVal: "Deep"},
				},
			},
		}
		runRoundTrip(t, msg, func() *Root { return &Root{} })
	})

	t.Run("Raw_Nested_Read_Modify_WriteBack", func(t *testing.T) {
		// Setup
		origin := &Root{
			RootId: 42,
			L1: &Level1{
				L1Data: "L1_Origin",
				L2: &Level2{
					Leaf: &Leaf{LeafId: 10, LeafVal: "Leaf_Origin"},
				},
			},
		}
		data, _ := origin.MarshalSymphony()

		var rootRaw RootRaw
		if err := rootRaw.UnmarshalSymphony(data); err != nil {
			t.Fatal(err)
		}

		// Test Root fields
		if rootRaw.GetRootId() != 42 {
			t.Error("RootId mismatch")
		}
		if err := rootRaw.SetRootId(100); err != nil {
			t.Fatal(err)
		}
		if rootRaw.GetRootId() != 100 {
			t.Error("RootId set failed")
		}

		// Read Down
		l1Raw := rootRaw.GetL1()
		if l1Raw == nil {
			t.Fatal("L1 is nil")
		}

		// Test Level1 fields (read only for public fields in nested messages)
		if l1Raw.GetL1Data() != "L1_Origin" {
			t.Error("L1Data mismatch")
		}
		// L1Data is public, so cannot be set on complete buffer (nested message)
		// Skipping L1Data mutation

		l2Raw := l1Raw.GetL2()
		if l2Raw == nil {
			t.Fatal("L2 is nil")
		}

		leafRaw := l2Raw.GetLeaf()
		if leafRaw == nil {
			t.Fatal("Leaf is nil")
		}

		// Test Leaf fields (read only - nested messages are complex with public/private split)
		if leafRaw.GetLeafId() != 10 {
			t.Error("LeafId mismatch")
		}
		if leafRaw.GetLeafVal() != "Leaf_Origin" {
			t.Error("LeafVal mismatch")
		}

		// Nested messages contain complete buffers, so:
		// - Can modify private fields (LeafVal)
		// - Cannot modify public fields (LeafId) - would panic
		if err := leafRaw.SetLeafVal("Leaf_Modified"); err != nil {
			t.Fatal(err)
		}
		if leafRaw.GetLeafVal() != "Leaf_Modified" {
			t.Error("LeafVal set failed")
		}

		// Note: Setting nested message fields (SetLeaf, SetL2, SetL1) is complex
		// with public/private split and requires remarshal logic - skipping for now
	})
}

func TestComplexMixed(t *testing.T) {
	t.Run("Struct_RoundTrip_Full", func(t *testing.T) {
		msg := &ComplexMixed{
			FInt32:         123,
			VString:        "Mixed",
			RInt64:         []int64{1, 2},
			NestedLeaf:     &Leaf{LeafVal: "Nested"},
			RString:        []string{"S1", "S2"},
			FBool:          true,
			RepeatedNested: []*Root{{RootId: 1, L1: &Level1{L1Data: "L1", L2: &Level2{Leaf: &Leaf{LeafId: 10, LeafVal: "Deep"}}}}},
			VBytes:         []byte{0x00},
		}
		runRoundTrip(t, msg, func() *ComplexMixed { return &ComplexMixed{} })
	})

	t.Run("Raw_Manipulation", func(t *testing.T) {
		// PUBLIC: v_string, nested_leaf, f_bool, v_bytes
		// PRIVATE: f_int32, r_int64, r_string, repeated_nested

		origin := &ComplexMixed{
			VString: "init",
			FBool:   false,
			VBytes:  []byte{1},
		}
		completeData, _ := origin.MarshalSymphony()

		// Test private fields on complete buffer
		var raw ComplexMixedRaw
		if err := raw.UnmarshalSymphony(completeData); err != nil {
			t.Fatal(err)
		}

		// Set private fields
		if err := raw.SetFInt32(999); err != nil {
			t.Fatal(err)
		}
		if err := raw.SetRInt64([]int64{5, 6, 7}); err != nil {
			t.Fatal(err)
		}
		if err := raw.SetRString([]string{"Str1", "Str2", "Str3"}); err != nil {
			t.Fatal(err)
		}

		// Set private repeated nested
		r1 := &Root{RootId: 1, L1: &Level1{L1Data: "R1"}}
		r1Bytes, _ := r1.MarshalSymphony()
		var r1Raw RootRaw
		r1Raw.UnmarshalSymphony(r1Bytes)

		r2 := &Root{RootId: 2, L1: &Level1{L1Data: "R2"}}
		r2Bytes, _ := r2.MarshalSymphony()
		var r2Raw RootRaw
		r2Raw.UnmarshalSymphony(r2Bytes)

		if err := raw.SetRepeatedNested([]RootRaw{r1Raw, r2Raw}); err != nil {
			t.Fatal(err)
		}

		// Verify private fields
		if raw.GetFInt32() != 999 {
			t.Error("FInt32 mismatch")
		}
		if !reflect.DeepEqual(raw.GetRInt64(), []int64{5, 6, 7}) {
			t.Error("RInt64 mismatch")
		}
		if !reflect.DeepEqual(raw.GetRString(), []string{"Str1", "Str2", "Str3"}) {
			t.Error("RString mismatch")
		}

		// Test public fields on public-only buffer
		offsetToPrivate := int(completeData[1]) | int(completeData[2])<<8 |
			int(completeData[3])<<16 | int(completeData[4])<<24
		publicOnly := completeData[:offsetToPrivate]

		// Directly assign public-only buffer (no unmarshal needed for Raw types)
		publicRaw := ComplexMixedRaw(publicOnly)

		// Set public fields
		if err := publicRaw.SetVString("NewString"); err != nil {
			t.Fatal(err)
		}
		if err := publicRaw.SetFBool(true); err != nil {
			t.Fatal(err)
		}
		if err := publicRaw.SetVBytes([]byte{0xAA, 0xBB, 0xCC}); err != nil {
			t.Fatal(err)
		}

		// Set public nested leaf
		l := &Leaf{LeafId: 42, LeafVal: "NewLeaf"}
		lBytes, _ := l.MarshalSymphony()
		var lRaw LeafRaw
		lRaw.UnmarshalSymphony(lBytes)
		if err := publicRaw.SetNestedLeaf(lRaw); err != nil {
			t.Fatal(err)
		}

		// Verify public fields
		if publicRaw.GetVString() != "NewString" {
			t.Error("VString mismatch")
		}
		if publicRaw.GetFBool() != true {
			t.Error("FBool mismatch")
		}
		if !bytes.Equal(publicRaw.GetVBytes(), []byte{0xAA, 0xBB, 0xCC}) {
			t.Error("VBytes mismatch")
		}

		// Verify public nested (on publicRaw since we set it there)
		nestedLeaf := publicRaw.GetNestedLeaf()
		if nestedLeaf == nil {
			t.Fatal("NestedLeaf is nil")
		}
		if nestedLeaf.GetLeafId() != 42 {
			t.Error("NestedLeaf LeafId mismatch")
		}
		if nestedLeaf.GetLeafVal() != "NewLeaf" {
			t.Error("NestedLeaf LeafVal mismatch")
		}

		// Verify private repeated nested (on raw since we set it there)
		repeatedNested := raw.GetRepeatedNested()
		if len(repeatedNested) != 2 {
			t.Errorf("RepeatedNested length mismatch: got %d, want 2", len(repeatedNested))
		}
		if repeatedNested[0].GetRootId() != 1 {
			t.Error("RepeatedNested[0] RootId mismatch")
		}
		if repeatedNested[0].GetL1().GetL1Data() != "R1" {
			t.Error("RepeatedNested[0] L1Data mismatch")
		}
		if repeatedNested[1].GetRootId() != 2 {
			t.Error("RepeatedNested[1] RootId mismatch")
		}
		if repeatedNested[1].GetL1().GetL1Data() != "R2" {
			t.Error("RepeatedNested[1] L1Data mismatch")
		}
	})
}

// Test public/private access control
func TestPublicPrivateAccessControl(t *testing.T) {
	// Create a message with both public and private fields
	msg := &Fixed{
		FInt32:  10,   // public
		FInt64:  20,   // private
		FUint32: 30,   // public
		FUint64: 40,   // private
		FBool:   true, // public
		FFloat:  1.5,  // private
		FDouble: 2.5,  // public
	}

	// Marshal to complete buffer
	completeBuffer, err := msg.MarshalSymphony()
	if err != nil {
		t.Fatalf("MarshalSymphony failed: %v", err)
	}

	// Get offset to private segment
	offsetToPrivate := int(completeBuffer[1]) | int(completeBuffer[2])<<8 |
		int(completeBuffer[3])<<16 | int(completeBuffer[4])<<24

	// Create public-only buffer by truncating
	publicOnlyBuffer := completeBuffer[:offsetToPrivate]

	t.Run("PublicGetters_WorkOnCompleteBuffer", func(t *testing.T) {
		var raw FixedRaw
		raw.UnmarshalSymphony(completeBuffer)

		// Public fields should be accessible
		if raw.GetFInt32() != 10 {
			t.Errorf("FInt32: got %d, want 10", raw.GetFInt32())
		}
		if raw.GetFUint32() != 30 {
			t.Errorf("FUint32: got %d, want 30", raw.GetFUint32())
		}
		if raw.GetFBool() != true {
			t.Error("FBool: got false, want true")
		}
		if raw.GetFDouble() != 2.5 {
			t.Errorf("FDouble: got %f, want 2.5", raw.GetFDouble())
		}
	})

	t.Run("PrivateGetters_WorkOnCompleteBuffer", func(t *testing.T) {
		var raw FixedRaw
		raw.UnmarshalSymphony(completeBuffer)

		// Private fields should be accessible
		if raw.GetFInt64() != 20 {
			t.Errorf("FInt64: got %d, want 20", raw.GetFInt64())
		}
		if raw.GetFUint64() != 40 {
			t.Errorf("FUint64: got %d, want 40", raw.GetFUint64())
		}
		if raw.GetFFloat() != 1.5 {
			t.Errorf("FFloat: got %f, want 1.5", raw.GetFFloat())
		}
	})

	t.Run("PublicGetters_WorkOnPublicOnlyBuffer", func(t *testing.T) {
		var raw FixedRaw
		raw.UnmarshalSymphony(publicOnlyBuffer)

		// Public fields should be accessible
		if raw.GetFInt32() != 10 {
			t.Errorf("FInt32: got %d, want 10", raw.GetFInt32())
		}
		if raw.GetFUint32() != 30 {
			t.Errorf("FUint32: got %d, want 30", raw.GetFUint32())
		}
	})

	t.Run("PrivateGetters_PanicOnPublicOnlyBuffer", func(t *testing.T) {
		var raw FixedRaw
		raw.UnmarshalSymphony(publicOnlyBuffer)

		// Private getters should panic
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when calling private getter on public-only buffer")
			}
		}()
		_ = raw.GetFInt64() // This should panic
	})

	t.Run("PublicSetters_PanicOnCompleteBuffer", func(t *testing.T) {
		var raw FixedRaw
		raw.UnmarshalSymphony(completeBuffer)

		// Public setters should panic on complete buffer
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when calling public setter on complete buffer")
			}
		}()
		_ = raw.SetFInt32(100) // This should panic
	})

	t.Run("PublicSetters_WorkOnPublicOnlyBuffer", func(t *testing.T) {
		var raw FixedRaw
		raw.UnmarshalSymphony(publicOnlyBuffer)

		// Public setters should work on public-only buffer
		err := raw.SetFInt32(100)
		if err != nil {
			t.Errorf("SetFInt32 failed: %v", err)
		}
		if raw.GetFInt32() != 100 {
			t.Errorf("After SetFInt32(100), got %d", raw.GetFInt32())
		}
	})

	t.Run("PrivateSetters_WorkOnCompleteBuffer", func(t *testing.T) {
		var raw FixedRaw
		raw.UnmarshalSymphony(completeBuffer)

		// Private setters should work on complete buffer
		err := raw.SetFInt64(200)
		if err != nil {
			t.Errorf("SetFInt64 failed: %v", err)
		}
		if raw.GetFInt64() != 200 {
			t.Errorf("After SetFInt64(200), got %d", raw.GetFInt64())
		}
	})

	t.Run("PrivateSetters_PanicOnPublicOnlyBuffer", func(t *testing.T) {
		var raw FixedRaw
		raw.UnmarshalSymphony(publicOnlyBuffer)

		// Private setters should panic on public-only buffer
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when calling private setter on public-only buffer")
			}
		}()
		_ = raw.SetFInt64(200) // This should panic
	})

	t.Run("UnmarshalSymphony_RequiresCompleteBuffer", func(t *testing.T) {
		// Struct Unmarshal should require complete buffer
		var msg2 Fixed
		err := msg2.UnmarshalSymphony(publicOnlyBuffer)
		if err == nil {
			t.Error("Expected error when unmarshaling public-only buffer to struct")
		}
	})
}
