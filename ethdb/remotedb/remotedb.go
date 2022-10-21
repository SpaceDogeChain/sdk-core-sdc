// Copyright 2022 The go-sdcereum Authors
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

// Package remotedb implements the key-value database layer based on a remote gsdc
// node. Under the hood, it utilises the `debug_dbGet` msdcod to implement a
// read-only database.
// There really are no guarantees in this database, since the local gsdc does not
// exclusive access, but it can be used for basic diagnostics of a remote node.
package remotedb

import (
	"github.com/sdcereum/go-sdcereum/common/hexutil"
	"github.com/sdcereum/go-sdcereum/sdcdb"
	"github.com/sdcereum/go-sdcereum/rpc"
)

// Database is a key-value lookup for a remote database via debug_dbGet.
type Database struct {
	remote *rpc.Client
}

func (db *Database) Has(key []byte) (bool, error) {
	if _, err := db.Get(key); err != nil {
		return false, nil
	}
	return true, nil
}

func (db *Database) Get(key []byte) ([]byte, error) {
	var resp hexutil.Bytes
	err := db.remote.Call(&resp, "debug_dbGet", hexutil.Bytes(key))
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (db *Database) HasAncient(kind string, number uint64) (bool, error) {
	if _, err := db.Ancient(kind, number); err != nil {
		return false, nil
	}
	return true, nil
}

func (db *Database) Ancient(kind string, number uint64) ([]byte, error) {
	var resp hexutil.Bytes
	err := db.remote.Call(&resp, "debug_dbAncient", kind, number)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (db *Database) AncientRange(kind string, start, count, maxBytes uint64) ([][]byte, error) {
	panic("not supported")
}

func (db *Database) Ancients() (uint64, error) {
	var resp uint64
	err := db.remote.Call(&resp, "debug_dbAncients")
	return resp, err
}

func (db *Database) Tail() (uint64, error) {
	panic("not supported")
}

func (db *Database) AncientSize(kind string) (uint64, error) {
	panic("not supported")
}

func (db *Database) ReadAncients(fn func(op sdcdb.AncientReaderOp) error) (err error) {
	return fn(db)
}

func (db *Database) Put(key []byte, value []byte) error {
	panic("not supported")
}

func (db *Database) Delete(key []byte) error {
	panic("not supported")
}

func (db *Database) ModifyAncients(f func(sdcdb.AncientWriteOp) error) (int64, error) {
	panic("not supported")
}

func (db *Database) TruncateHead(n uint64) error {
	panic("not supported")
}

func (db *Database) TruncateTail(n uint64) error {
	panic("not supported")
}

func (db *Database) Sync() error {
	return nil
}

func (db *Database) MigrateTable(s string, f func([]byte) ([]byte, error)) error {
	panic("not supported")
}

func (db *Database) NewBatch() sdcdb.Batch {
	panic("not supported")
}

func (db *Database) NewBatchWithSize(size int) sdcdb.Batch {
	panic("not supported")
}

func (db *Database) NewIterator(prefix []byte, start []byte) sdcdb.Iterator {
	panic("not supported")
}

func (db *Database) Stat(property string) (string, error) {
	panic("not supported")
}

func (db *Database) AncientDatadir() (string, error) {
	panic("not supported")
}

func (db *Database) Compact(start []byte, limit []byte) error {
	return nil
}

func (db *Database) NewSnapshot() (sdcdb.Snapshot, error) {
	panic("not supported")
}

func (db *Database) Close() error {
	db.remote.Close()
	return nil
}

func New(client *rpc.Client) sdcdb.Database {
	return &Database{
		remote: client,
	}
}
