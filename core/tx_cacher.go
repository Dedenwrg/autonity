// Copyright 2018 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
    "runtime"
    "sync/atomic"

    "github.com/autonity/autonity/core/types"
)

// txSenderCacherRequest is a request for recovering transaction senders with a
// specific signature scheme and caching it into the transactions themselves.
//
// The inc field defines the number of transactions to skip after each recovery,
// which is used to feed the same underlying input array to different threads but
// ensure they process the early transactions fast.
type txSenderCacherRequest struct {
    signer types.Signer
    txs    []*types.Transaction
    inc    int
}

// TxSenderCacher is a helper structure to concurrently ecrecover transaction
// senders from digital signatures on background threads.
type TxSenderCacher struct {
    threads  int
    isClosed *uint32
    tasks    chan *txSenderCacherRequest
}

// NewTxSenderCacher creates a new transaction sender background cacher and starts
// as many processing goroutines as allowed by the GOMAXPROCS on construction.
func NewTxSenderCacher(threads ...int) *TxSenderCacher {
    num := 0
    if len(threads) == 0 {
        num = runtime.NumCPU()
    } else {
        num = threads[0]
    }
    cacher := &TxSenderCacher{
        tasks:    make(chan *txSenderCacherRequest, 3*num),
        isClosed: new(uint32),
        threads:  num,
    }
    for i := 0; i < num; i++ {
        go cacher.cache()
    }
    return cacher
}

// cache is an infinite loop, caching transaction senders from various forms of
// data structures.
func (cacher *TxSenderCacher) cache() {
    for task := range cacher.tasks {
        for i := 0; i < len(task.txs); i += task.inc {
            types.Sender(task.signer, task.txs[i])
        }
    }
}

// recover recovers the senders from a batch of transactions and caches them
// back into the same data structures. There is no validation being done, nor
// any reaction to invalid signatures. That is up to calling code later.
func (cacher *TxSenderCacher) recover(signer types.Signer, txs []*types.Transaction) {
    // If there's nothing to recover, abort
    if len(txs) == 0 {
        return
    }
    // Ensure we have meaningful task sizes and schedule the recoveries
    tasks := cacher.threads
    if len(txs) < tasks*4 {
        tasks = (len(txs) + 3) / 4
    }
    for i := 0; i < tasks; i++ {
        if atomic.LoadUint32(cacher.isClosed) == 1 {
            return
        }

        cacher.tasks <- &txSenderCacherRequest{
            signer: signer,
            txs:    txs[i:],
            inc:    tasks,
        }
    }
}

// recoverFromBlocks recovers the senders from a batch of blocks and caches them
// back into the same data structures. There is no validation being done, nor
// any reaction to invalid signatures. That is up to calling code later.
func (cacher *TxSenderCacher) recoverFromBlocks(signer types.Signer, blocks []*types.Block) {
    count := 0
    for _, block := range blocks {
        count += len(block.Transactions())
    }
    txs := make([]*types.Transaction, 0, count)
    for _, block := range blocks {
        txs = append(txs, block.Transactions()...)
    }
    cacher.recover(signer, txs)
}

func (cacher *TxSenderCacher) Close() {
    if atomic.CompareAndSwapUint32(cacher.isClosed, 0, 1) {
        close(cacher.tasks)
    }
}
