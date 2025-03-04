// Copyright 2015 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package raft

import (
	"fmt"
	"github.com/pingcap-incubator/tinykv/log"
	pb "github.com/pingcap-incubator/tinykv/proto/pkg/eraftpb"
)

// RaftLog manage the log entries, its struct look like:
//
//	snapshot/first.....applied....committed....stabled.....last
//	--------|------------------------------------------------|
//	                          log entries
//
// for simplify the RaftLog implement should manage all log entries
// that not truncated
type RaftLog struct {
	// storage contains all stable entries since the last snapshot.
	storage Storage

	// committed is the highest log position that is known to be in
	// stable storage on a quorum of nodes.
	committed uint64

	// applied is the highest log position that the application has
	// been instructed to apply to its state machine.
	// Invariant: applied <= committed
	applied uint64

	// log entries with index <= stabled are persisted to storage.
	// It is used to record the logs that are not persisted by storage yet.
	// Everytime handling `Ready`, the unstabled logs will be included.
	stabled uint64

	// all entries that have not yet compact.
	entries []pb.Entry

	// the incoming unstable snapshot, if any.
	// (Used in 2C)
	pendingSnapshot *pb.Snapshot

	// Your Data Here (2A).
	start uint64
}

// newLog returns log using the given storage. It recovers the log
// to the state that it just commits and applies the latest snapshot.
func newLog(storage Storage) *RaftLog {
	// Your Code Here (2A).
	log := &RaftLog{}
	log.storage = storage
	state, _, err := log.storage.InitialState()
	log.committed = state.Commit
	mustBeNil(err)
	start, err := storage.FirstIndex()
	log.start = start - 1
	log.applied = log.start
	mustBeNil(err)
	LastIndex, err := storage.LastIndex()
	log.stabled = LastIndex
	mustBeNil(err)

	entries, err := storage.Entries(start, LastIndex+1)
	mustBeNil(err)
	//todo(judge start)
	log.entries = make([]pb.Entry, 1) // contain start
	log.entries[0].Index = log.start
	log.entries[0].Term, err = storage.Term(log.start)
	mustBeNil(err)
	log.entries = append(log.entries, entries...)

	fmt.Printf("newLog: %+v\n", log)
	return log
}

// We need to compact the log entries in some point of time like
// storage compact stabled log entries prevent the log entries
// grow unlimitedly in memory
func (l *RaftLog) maybeCompact() {
	// Your Code Here (2C).
}

// allEntries return all the entries not compacted.
// note, exclude any dummy entries from the return value.
// note, this is one of the test stub functions you need to implement.
func (l *RaftLog) allEntries() []pb.Entry {
	// Your Code Here (2A).
	return l.entries[1:]
}

// unstableEntries return all the unstable entries
func (l *RaftLog) unstableEntries() []pb.Entry {
	if l.stabled == l.LastIndex() {
		return []pb.Entry{}
	}
	// Your Code Here (2A).
	log.Debugf("unstableEntries: start: %d, stabled: %d, len: %d\n", l.start, l.stabled, len(l.entries))
	return l.entries[l.stabled-l.start+1:]
}

// nextEnts returns all the committed but not applied entries
func (l *RaftLog) nextEnts() (ents []pb.Entry) {
	if l.applied == l.committed {
		return []pb.Entry{}
	}

	if l.applied > l.committed {
		log.Panicf("applied(%d) > committed(%d)]", l.applied, l.committed)
	}
	ents = l.entries[l.applied-l.start+1 : l.committed-l.start+1]
	log.Errorf("nextEnts: %v", ents)
	return
}

// LastIndex return the last index of the log entries
func (l *RaftLog) LastIndex() uint64 {
	return l.LastLog().Index
}

func (l *RaftLog) NextIndex() uint64 {
	return l.LastLog().Index + 1
}

// LastTerm return the last Term of the log entries
func (l *RaftLog) LastTerm() uint64 {
	return l.LastLog().Term
}

// Term return the term of the entry in the given index
func (l *RaftLog) Term(i uint64) (uint64, error) {
	// Your Code Here (2A).
	at, err := l.entryAt(i)
	if err != nil {
		return 0, err
	}
	return at.Term, nil
}
func (l *RaftLog) LastLog() pb.Entry {
	return l.entries[len(l.entries)-1]
}

// if not in [First,LastLogIndex] return nil
func (l *RaftLog) entryAt(index uint64) (*pb.Entry, error) {
	if index == l.start {
		return &l.entries[0], nil
	}

	if index < l.First() { // is compact
		return nil, ErrCompacted
	}
	if index > l.LastIndex() { // exceed
		return nil, ErrUnavailable
	}

	//[First,Last]
	return &l.entries[index-l.start], nil
}
func (l *RaftLog) append(entries ...pb.Entry) uint64 {
	l.entries = append(l.entries, entries...)
	return l.LastIndex()
}

// [lo,hi]
func (l *RaftLog) slice(lo, hi uint64) []*pb.Entry {
	if hi < lo {
		return nil
	}
	if hi < l.First() {
		return nil //
	}
	lo = max(lo, l.First())
	hi = max(hi, l.First())
	hi = min(hi, l.LastIndex())

	lo, hi = lo-l.start, hi-l.start
	return covEntry2PtrC(l.entries[lo : hi+1]...)
}

func (l *RaftLog) First() uint64 {
	return l.start + 1
}

// use in append log, return log is in [First,LastLogIndex]
func (l *RaftLog) Contain(index uint64) bool {
	return l.First() <= index && index <= l.LastIndex()
}

func (l *RaftLog) IsConflict(index, term uint64) bool {
	// not contain this log or if term not equal,is conflict should truncate
	return !l.Contain(index) || l.entries[index-l.start].Term != term
}

// truncate index to end(include index)
func (l *RaftLog) truncate(index uint64) {
	log.Infof("truncate: %d", index)
	l.entries = l.entries[:index-l.start]
	l.stabled = min(l.stabled, l.LastIndex())
}

func (l *RaftLog) updateCommitIndex(commit uint64) {
	if commit < l.committed {
		return
	}
	l.committed = commit
}

// cutDown cut down the log entries to (index,LastLogIndex]
func (l *RaftLog) cutDown(index, term uint64) {
	var cp []pb.Entry
	if index > l.LastIndex() {
		log.Warnf("cutDown: index(%d) > LastIndex(%d)", index, l.LastIndex())
		cp = make([]pb.Entry, 1)
	} else {
		cp = make([]pb.Entry, l.LastIndex()-index-l.start+1)
		log.Infof("cut down: %d, %d, %d, %d", index, l.LastIndex(), len(l.entries), len(cp))
	}
	cp[0].Index, cp[0].Term = index, term
	if index+1 < l.LastIndex() {
		copy(cp[1:], l.entries[index+1:])
	}
	l.entries = cp
	l.start = index

	l.committed = max(l.committed, index)
	l.applied = max(l.applied, index)
	l.stabled = max(l.stabled, index)
}
