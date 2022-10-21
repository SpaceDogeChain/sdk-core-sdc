// Copyright 2018 The go-sdcereum Authors
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

package abi

import (
	"strings"
	"testing"
)

const msdcoddata = `
[
	{"type": "function", "name": "balance", "stateMutability": "view"},
	{"type": "function", "name": "send", "inputs": [{ "name": "amount", "type": "uint256" }]},
	{"type": "function", "name": "transfer", "inputs": [{"name": "from", "type": "address"}, {"name": "to", "type": "address"}, {"name": "value", "type": "uint256"}], "outputs": [{"name": "success", "type": "bool"}]},
	{"constant":false,"inputs":[{"components":[{"name":"x","type":"uint256"},{"name":"y","type":"uint256"}],"name":"a","type":"tuple"}],"name":"tuple","outputs":[],"payable":false,"stateMutability":"nonpayable","type":"function"},
	{"constant":false,"inputs":[{"components":[{"name":"x","type":"uint256"},{"name":"y","type":"uint256"}],"name":"a","type":"tuple[]"}],"name":"tupleSlice","outputs":[],"payable":false,"stateMutability":"nonpayable","type":"function"},
	{"constant":false,"inputs":[{"components":[{"name":"x","type":"uint256"},{"name":"y","type":"uint256"}],"name":"a","type":"tuple[5]"}],"name":"tupleArray","outputs":[],"payable":false,"stateMutability":"nonpayable","type":"function"},
	{"constant":false,"inputs":[{"components":[{"name":"x","type":"uint256"},{"name":"y","type":"uint256"}],"name":"a","type":"tuple[5][]"}],"name":"complexTuple","outputs":[],"payable":false,"stateMutability":"nonpayable","type":"function"},
	{"stateMutability":"nonpayable","type":"fallback"},
	{"stateMutability":"payable","type":"receive"}
]`

func TestMsdcodString(t *testing.T) {
	var table = []struct {
		msdcod      string
		expectation string
	}{
		{
			msdcod:      "balance",
			expectation: "function balance() view returns()",
		},
		{
			msdcod:      "send",
			expectation: "function send(uint256 amount) returns()",
		},
		{
			msdcod:      "transfer",
			expectation: "function transfer(address from, address to, uint256 value) returns(bool success)",
		},
		{
			msdcod:      "tuple",
			expectation: "function tuple((uint256,uint256) a) returns()",
		},
		{
			msdcod:      "tupleArray",
			expectation: "function tupleArray((uint256,uint256)[5] a) returns()",
		},
		{
			msdcod:      "tupleSlice",
			expectation: "function tupleSlice((uint256,uint256)[] a) returns()",
		},
		{
			msdcod:      "complexTuple",
			expectation: "function complexTuple((uint256,uint256)[5][] a) returns()",
		},
		{
			msdcod:      "fallback",
			expectation: "fallback() returns()",
		},
		{
			msdcod:      "receive",
			expectation: "receive() payable returns()",
		},
	}

	abi, err := JSON(strings.NewReader(msdcoddata))
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range table {
		var got string
		if test.msdcod == "fallback" {
			got = abi.Fallback.String()
		} else if test.msdcod == "receive" {
			got = abi.Receive.String()
		} else {
			got = abi.Msdcods[test.msdcod].String()
		}
		if got != test.expectation {
			t.Errorf("expected string to be %s, got %s", test.expectation, got)
		}
	}
}

func TestMsdcodSig(t *testing.T) {
	var cases = []struct {
		msdcod string
		expect string
	}{
		{
			msdcod: "balance",
			expect: "balance()",
		},
		{
			msdcod: "send",
			expect: "send(uint256)",
		},
		{
			msdcod: "transfer",
			expect: "transfer(address,address,uint256)",
		},
		{
			msdcod: "tuple",
			expect: "tuple((uint256,uint256))",
		},
		{
			msdcod: "tupleArray",
			expect: "tupleArray((uint256,uint256)[5])",
		},
		{
			msdcod: "tupleSlice",
			expect: "tupleSlice((uint256,uint256)[])",
		},
		{
			msdcod: "complexTuple",
			expect: "complexTuple((uint256,uint256)[5][])",
		},
	}
	abi, err := JSON(strings.NewReader(msdcoddata))
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range cases {
		got := abi.Msdcods[test.msdcod].Sig
		if got != test.expect {
			t.Errorf("expected string to be %s, got %s", test.expect, got)
		}
	}
}
