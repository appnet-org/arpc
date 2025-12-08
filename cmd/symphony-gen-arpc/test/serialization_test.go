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
		// 1. Create bytes via Struct
		origin := &Fixed{}
		data, _ := origin.MarshalSymphony()

		// 2. Unmarshal into Raw
		var raw FixedRaw
		if err := raw.UnmarshalSymphony(data); err != nil {
			t.Fatalf("UnmarshalSymphony failed: %v", err)
		}

		// 3. Mutate All Fields
		if err := raw.SetFInt32(math.MinInt32); err != nil {
			t.Fatal(err)
		}
		if err := raw.SetFInt64(math.MinInt64); err != nil {
			t.Fatal(err)
		}
		if err := raw.SetFUint32(math.MaxUint32); err != nil {
			t.Fatal(err)
		}
		if err := raw.SetFUint64(math.MaxUint64); err != nil {
			t.Fatal(err)
		}
		if err := raw.SetFBool(true); err != nil {
			t.Fatal(err)
		}
		if err := raw.SetFFloat(1.234); err != nil {
			t.Fatal(err)
		}
		if err := raw.SetFDouble(5.6789); err != nil {
			t.Fatal(err)
		}

		// 4. Verify All Fields
		if got := raw.GetFInt32(); got != math.MinInt32 {
			t.Errorf("FInt32: %v", got)
		}
		if got := raw.GetFInt64(); got != math.MinInt64 {
			t.Errorf("FInt64: %v", got)
		}
		if got := raw.GetFUint32(); got != math.MaxUint32 {
			t.Errorf("FUint32: %v", got)
		}
		if got := raw.GetFUint64(); got != math.MaxUint64 {
			t.Errorf("FUint64: %v", got)
		}
		if got := raw.GetFBool(); !got {
			t.Errorf("FBool: %v", got)
		}
		if got := raw.GetFFloat(); got != 1.234 {
			t.Errorf("FFloat: %v", got)
		}
		if got := raw.GetFDouble(); got != 5.6789 {
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
		origin := &Var{VString: "init", VBytes: []byte{}}
		data, _ := origin.MarshalSymphony()

		var raw VarRaw
		if err := raw.UnmarshalSymphony(data); err != nil {
			t.Fatal(err)
		}

		// String Mutation
		if err := raw.SetVString("modified_string"); err != nil {
			t.Fatal(err)
		}
		if raw.GetVString() != "modified_string" {
			t.Error("String mismatch")
		}

		// Bytes Mutation
		newBytes := []byte{1, 2, 3, 4}
		if err := raw.SetVBytes(newBytes); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(raw.GetVBytes(), newBytes) {
			t.Error("Bytes mismatch")
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
		// Initialize empty
		origin := &RepeatedFixed{}
		data, _ := origin.MarshalSymphony()
		var raw RepeatedFixedRaw
		if err := raw.UnmarshalSymphony(data); err != nil {
			t.Fatal(err)
		}

		// Test Int32
		vInt32 := []int32{1, 2, 3}
		if err := raw.SetRInt32(vInt32); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(raw.GetRInt32(), vInt32) {
			t.Error("RInt32 mismatch")
		}

		// Test Int64
		vInt64 := []int64{10, 20, 30}
		if err := raw.SetRInt64(vInt64); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(raw.GetRInt64(), vInt64) {
			t.Error("RInt64 mismatch")
		}

		// Test Uint32
		vUint32 := []uint32{5, 6, 7}
		if err := raw.SetRUint32(vUint32); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(raw.GetRUint32(), vUint32) {
			t.Error("RUint32 mismatch")
		}

		// Test Uint64
		vUint64 := []uint64{50, 60, 70}
		if err := raw.SetRUint64(vUint64); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(raw.GetRUint64(), vUint64) {
			t.Error("RUint64 mismatch")
		}

		// Test Float
		vFloat := []float32{1.1, 2.2}
		if err := raw.SetRFloat(vFloat); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(raw.GetRFloat(), vFloat) {
			t.Error("RFloat mismatch")
		}

		// Test Double
		vDouble := []float64{1.11, 2.22}
		if err := raw.SetRDouble(vDouble); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(raw.GetRDouble(), vDouble) {
			t.Error("RDouble mismatch")
		}

		// Test Bool
		vBool := []bool{true, false, true}
		if err := raw.SetRBool(vBool); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(raw.GetRBool(), vBool) {
			t.Error("RBool mismatch")
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
		origin := &RepeatedVar{}
		data, _ := origin.MarshalSymphony()
		var raw RepeatedVarRaw
		if err := raw.UnmarshalSymphony(data); err != nil {
			t.Fatal(err)
		}

		// Strings
		strs := []string{"A", "B", "C"}
		if err := raw.SetRString(strs); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(raw.GetRString(), strs) {
			t.Error("RString mismatch")
		}

		// Bytes
		bs := [][]byte{{0x01}, {0x02, 0x03}}
		if err := raw.SetRBytes(bs); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(raw.GetRBytes(), bs) {
			t.Error("RBytes mismatch")
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

		// Test Level1 fields
		if l1Raw.GetL1Data() != "L1_Origin" {
			t.Error("L1Data mismatch")
		}
		if err := l1Raw.SetL1Data("L1_Modified"); err != nil {
			t.Fatal(err)
		}
		if l1Raw.GetL1Data() != "L1_Modified" {
			t.Error("L1Data set failed")
		}

		l2Raw := l1Raw.GetL2()
		if l2Raw == nil {
			t.Fatal("L2 is nil")
		}

		leafRaw := l2Raw.GetLeaf()
		if leafRaw == nil {
			t.Fatal("Leaf is nil")
		}

		// Test Leaf fields
		if leafRaw.GetLeafId() != 10 {
			t.Error("LeafId mismatch")
		}
		if leafRaw.GetLeafVal() != "Leaf_Origin" {
			t.Fatal("Read failed")
		}
		if err := leafRaw.SetLeafId(99); err != nil {
			t.Fatal(err)
		}
		if err := leafRaw.SetLeafVal("Leaf_Modified"); err != nil {
			t.Fatal(err)
		}
		if leafRaw.GetLeafId() != 99 {
			t.Error("LeafId set failed")
		}

		// Write Back Up
		if err := l2Raw.SetLeaf(leafRaw); err != nil {
			t.Fatal(err)
		}
		if err := l1Raw.SetL2(l2Raw); err != nil {
			t.Fatal(err)
		}
		if err := rootRaw.SetL1(l1Raw); err != nil {
			t.Fatal(err)
		}

		// Verify
		if rootRaw.GetL1().GetL2().GetLeaf().GetLeafVal() != "Leaf_Modified" {
			t.Error("Modify failed")
		}
		if rootRaw.GetL1().GetL2().GetLeaf().GetLeafId() != 99 {
			t.Error("LeafId modify failed")
		}
		if rootRaw.GetL1().GetL1Data() != "L1_Modified" {
			t.Error("L1Data modify failed")
		}
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
		// Setup
		origin := &ComplexMixed{}
		data, _ := origin.MarshalSymphony()
		var raw ComplexMixedRaw
		if err := raw.UnmarshalSymphony(data); err != nil {
			t.Fatal(err)
		}

		// Set Fixed fields
		if err := raw.SetFInt32(999); err != nil {
			t.Fatal(err)
		}
		if err := raw.SetFBool(true); err != nil {
			t.Fatal(err)
		}

		// Set Variable fields
		if err := raw.SetVString("NewString"); err != nil {
			t.Fatal(err)
		}
		if err := raw.SetVBytes([]byte{0xAA, 0xBB, 0xCC}); err != nil {
			t.Fatal(err)
		}

		// Set Repeated Fixed fields
		if err := raw.SetRInt64([]int64{5, 6, 7}); err != nil {
			t.Fatal(err)
		}

		// Set Repeated Variable fields
		if err := raw.SetRString([]string{"Str1", "Str2", "Str3"}); err != nil {
			t.Fatal(err)
		}

		// Set Nested (singular)
		// Create Raw Leaf via Struct->Marshal
		l := &Leaf{LeafId: 42, LeafVal: "NewLeaf"}
		lBytes, _ := l.MarshalSymphony()
		var lRaw LeafRaw
		lRaw.UnmarshalSymphony(lBytes)

		if err := raw.SetNestedLeaf(lRaw); err != nil {
			t.Fatal(err)
		}

		// Set Repeated Nested
		// Create Raw Root via Struct->Marshal
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

		// Verify Fixed fields
		if raw.GetFInt32() != 999 {
			t.Error("FInt32 mismatch")
		}
		if raw.GetFBool() != true {
			t.Error("FBool mismatch")
		}

		// Verify Variable fields
		if raw.GetVString() != "NewString" {
			t.Error("VString mismatch")
		}
		if !bytes.Equal(raw.GetVBytes(), []byte{0xAA, 0xBB, 0xCC}) {
			t.Error("VBytes mismatch")
		}

		// Verify Repeated Fixed fields
		if !reflect.DeepEqual(raw.GetRInt64(), []int64{5, 6, 7}) {
			t.Error("RInt64 mismatch")
		}

		// Verify Repeated Variable fields
		if !reflect.DeepEqual(raw.GetRString(), []string{"Str1", "Str2", "Str3"}) {
			t.Error("RString mismatch")
		}

		// Verify Nested (singular)
		nestedLeaf := raw.GetNestedLeaf()
		if nestedLeaf == nil {
			t.Fatal("NestedLeaf is nil")
		}
		if nestedLeaf.GetLeafId() != 42 {
			t.Error("NestedLeaf LeafId mismatch")
		}
		if nestedLeaf.GetLeafVal() != "NewLeaf" {
			t.Error("NestedLeaf LeafVal mismatch")
		}

		// Verify Repeated Nested
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
