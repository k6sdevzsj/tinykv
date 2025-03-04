package raft

import (
	"errors"
	"github.com/pingcap-incubator/tinykv/log"
	pb "github.com/pingcap-incubator/tinykv/proto/pkg/eraftpb"
)

func (r *Raft) NewHeartbeatMsg(to uint64) pb.Message {
	if r.State != StateLeader {
		log.Panicf("you state %s not leader", r.info())
	}
	return pb.Message{
		MsgType: pb.MessageType_MsgHeartbeat,
		To:      to,
		Commit:  r.RaftLog.committed,
	}
}
func (r *Raft) NewRespHeartbeatMsg(to uint64) pb.Message {
	return pb.Message{
		MsgType: pb.MessageType_MsgHeartbeatResponse,
		To:      to,
	}
}

func (r *Raft) NewRequestVoteMsg(to uint64) pb.Message {
	if r.State != StateCandidate {
		log.Panicf("you state %s not candidate", r.info())
	}
	var LastLog = r.RaftLog.LastLog()
	return pb.Message{
		MsgType: pb.MessageType_MsgRequestVote,
		To:      to,
		LogTerm: LastLog.Term,
		Index:   LastLog.Index,
	}
}
func (r *Raft) NewRespVoteMsg(to uint64, reject bool) pb.Message {
	return pb.Message{
		MsgType: pb.MessageType_MsgRequestVoteResponse,
		To:      to,
		From:    r.id,
		Reject:  reject,
	}
}

func (r *Raft) NewAppendMsg(to uint64) pb.Message {
	if r.State != StateLeader {
		log.Panicf("you state %s not leader", r.info())
	}
	pr, ok := r.Prs[to]
	if !ok {
		log.Panicf("don't have this node %d ?", pr)
	}

	prevLog, err := r.RaftLog.entryAt(pr.Next - 1)

	if err != nil {
		if errors.Is(err, ErrCompacted) {
			log.Infof("%s send to %d {%d:%d} compacted", r.info(), to, pr.Next, r.RaftLog.LastIndex())
			storage := r.RaftLog.storage
			snapshot, err := storage.Snapshot()
			if err != nil {
				if errors.Is(err, ErrSnapshotTemporarilyUnavailable) {
					log.Errorf("%s send to %d {%d:%d} snapshot temporarily unavailable", r.info(), to, pr.Next, r.RaftLog.LastIndex())
					return r.NewHeartbeatMsg(to)
				}
				log.Panicf("%s send to %d {%d:%d} snapshot error %s", r.info(), to, pr.Next, r.RaftLog.LastIndex(), err)
			}
			return pb.Message{
				MsgType:  pb.MessageType_MsgSnapshot,
				To:       to,
				Snapshot: &snapshot,
			}
		}
		// todo: send snap?
		log.Panic(r.info(), "send to ", to, " can not get next ", err)
	}
	log.Infof("%s send log to %d {%d:%d}", r.info(), to, pr.Next, r.RaftLog.LastIndex())

	return pb.Message{
		MsgType: pb.MessageType_MsgAppend,
		To:      to,
		Index:   prevLog.Index,
		LogTerm: prevLog.Term,
		Commit:  r.RaftLog.committed,
		Entries: r.RaftLog.slice(pr.Next, r.RaftLog.LastIndex()),
	}
}
func (r *Raft) NewRespAppendMsg(to, index uint64, reject bool) pb.Message {
	return pb.Message{
		MsgType: pb.MessageType_MsgAppendResponse,
		To:      to,
		Reject:  reject,
		Index:   index,
	}
}
