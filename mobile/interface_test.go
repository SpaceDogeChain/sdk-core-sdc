// Copyright 2019 The go-sdcereum Authors
// This file is part of the go-sdcereum library.
//
// The go-sdcereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-sdcereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-sdcereum library. If not, see <http://www.gnu.org/licenses/>.

package gsdc

import (
	"fmt"
	"math/big"
	"reflect"
	"testing"

	"github.com/sdcereum/go-sdcereum/common"
)

func TestInterfaceGetSet(t *testing.T) {
	var tests = []struct {
		msdcod string
		input  interface{}
		expect interface{}
	}{
		{"Bool", true, true},
		{"Bool", false, false},
		{"Bools", &Bools{[]bool{false, true}}, &Bools{[]bool{false, true}}},
		{"String", "go-sdcereum", "go-sdcereum"},
		{"Strings", &Strings{strs: []string{"hello", "world"}}, &Strings{strs: []string{"hello", "world"}}},
		{"Binary", []byte{0x01, 0x02}, []byte{0x01, 0x02}},
		{"Binaries", &Binaries{[][]byte{{0x01, 0x02}, {0x03, 0x04}}}, &Binaries{[][]byte{{0x01, 0x02}, {0x03, 0x04}}}},
		{"Address", &Address{common.HexToAddress("deadbeef")}, &Address{common.HexToAddress("deadbeef")}},
		{"Addresses", &Addresses{[]common.Address{common.HexToAddress("deadbeef"), common.HexToAddress("cafebabe")}}, &Addresses{[]common.Address{common.HexToAddress("deadbeef"), common.HexToAddress("cafebabe")}}},
		{"Hash", &Hash{common.HexToHash("deadbeef")}, &Hash{common.HexToHash("deadbeef")}},
		{"Hashes", &Hashes{[]common.Hash{common.HexToHash("deadbeef"), common.HexToHash("cafebabe")}}, &Hashes{[]common.Hash{common.HexToHash("deadbeef"), common.HexToHash("cafebabe")}}},
		{"Int8", int8(1), int8(1)},
		{"Int16", int16(1), int16(1)},
		{"Int32", int32(1), int32(1)},
		{"Int64", int64(1), int64(1)},
		{"Int8s", &BigInts{[]*big.Int{big.NewInt(1), big.NewInt(2)}}, &BigInts{[]*big.Int{big.NewInt(1), big.NewInt(2)}}},
		{"Int16s", &BigInts{[]*big.Int{big.NewInt(1), big.NewInt(2)}}, &BigInts{[]*big.Int{big.NewInt(1), big.NewInt(2)}}},
		{"Int32s", &BigInts{[]*big.Int{big.NewInt(1), big.NewInt(2)}}, &BigInts{[]*big.Int{big.NewInt(1), big.NewInt(2)}}},
		{"Int64s", &BigInts{[]*big.Int{big.NewInt(1), big.NewInt(2)}}, &BigInts{[]*big.Int{big.NewInt(1), big.NewInt(2)}}},
		{"Uint8", NewBigInt(1), NewBigInt(1)},
		{"Uint16", NewBigInt(1), NewBigInt(1)},
		{"Uint32", NewBigInt(1), NewBigInt(1)},
		{"Uint64", NewBigInt(1), NewBigInt(1)},
		{"Uint8s", &BigInts{[]*big.Int{big.NewInt(1), big.NewInt(2)}}, &BigInts{[]*big.Int{big.NewInt(1), big.NewInt(2)}}},
		{"Uint16s", &BigInts{[]*big.Int{big.NewInt(1), big.NewInt(2)}}, &BigInts{[]*big.Int{big.NewInt(1), big.NewInt(2)}}},
		{"Uint32s", &BigInts{[]*big.Int{big.NewInt(1), big.NewInt(2)}}, &BigInts{[]*big.Int{big.NewInt(1), big.NewInt(2)}}},
		{"Uint64s", &BigInts{[]*big.Int{big.NewInt(1), big.NewInt(2)}}, &BigInts{[]*big.Int{big.NewInt(1), big.NewInt(2)}}},
		{"BigInt", NewBigInt(1), NewBigInt(1)},
		{"BigInts", &BigInts{[]*big.Int{big.NewInt(1), big.NewInt(2)}}, &BigInts{[]*big.Int{big.NewInt(1), big.NewInt(2)}}},
	}

	args := NewInterfaces(len(tests))

	callFn := func(receiver interface{}, msdcod string, arg interface{}) interface{} {
		rval := reflect.ValueOf(receiver)
		rval.MsdcodByName(fmt.Sprintf("Set%s", msdcod)).Call([]reflect.Value{reflect.ValueOf(arg)})
		res := rval.MsdcodByName(fmt.Sprintf("Get%s", msdcod)).Call(nil)
		if len(res) > 0 {
			return res[0].Interface()
		}
		return nil
	}

	for index, c := range tests {
		// In theory the change of iface shouldn't effect the args value
		iface, _ := args.Get(index)
		result := callFn(iface, c.msdcod, c.input)
		if !reflect.DeepEqual(result, c.expect) {
			t.Errorf("Interface get/set mismatch, want %v, got %v", c.expect, result)
		}
		// Check whsdcer the underlying value in args is still zero
		iface, _ = args.Get(index)
		if iface.object != nil {
			t.Error("Get operation is not write safe")
		}
	}
}
