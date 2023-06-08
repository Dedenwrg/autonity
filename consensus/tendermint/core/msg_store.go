package core

import (
	"github.com/autonity/autonity/common"
	mUtils "github.com/autonity/autonity/consensus/tendermint/core/messageutils"
	"sync"
)

var NilValue = common.Hash{}

type MsgStore struct {
	sync.RWMutex
	// the first height that msg are buffered from after node is start.
	firstHeight uint64
	// map[Height]map[Round]map[MsgType]map[common.address][]*Message
	messages map[uint64]map[int64]map[uint8]map[common.Address][]*mUtils.Message
}

func NewMsgStore() *MsgStore {
	return &MsgStore{
		RWMutex:     sync.RWMutex{},
		firstHeight: uint64(0),
		messages:    make(map[uint64]map[int64]map[uint8]map[common.Address][]*mUtils.Message)}
}

// Save store msg into msg store
func (ms *MsgStore) Save(m *mUtils.Message) {
	ms.Lock()
	defer ms.Unlock()

	if ms.firstHeight == uint64(0) {
		ms.firstHeight = m.H()
	}

	height, _ := m.Height()
	roundMap, ok := ms.messages[height.Uint64()]
	if !ok {
		roundMap = make(map[int64]map[uint8]map[common.Address][]*mUtils.Message)
		ms.messages[height.Uint64()] = roundMap
	}

	round, _ := m.Round()
	msgTypeMap, ok := roundMap[round]
	if !ok {
		msgTypeMap = make(map[uint8]map[common.Address][]*mUtils.Message)
		roundMap[round] = msgTypeMap
	}

	addressMap, ok := msgTypeMap[m.Code]
	if !ok {
		addressMap = make(map[common.Address][]*mUtils.Message)
		msgTypeMap[m.Code] = addressMap
	}

	msgs, ok := addressMap[m.Address]
	if !ok {
		var msgList []*mUtils.Message
		addressMap[m.Address] = append(msgList, m)
		return
	}
	addressMap[m.Address] = append(msgs, m)
}

func (ms *MsgStore) FirstHeightBuffered() uint64 {
	ms.Lock()
	defer ms.Unlock()
	return ms.firstHeight
}

func (ms *MsgStore) DeleteMsgsBeforeHeight(height uint64) {
	ms.Lock()
	defer ms.Unlock()
	for h := range ms.messages {
		if h <= height {
			// Delete map entry for this height
			delete(ms.messages, h)
		}
	}
}

// RemoveMsg only used for integration tests.
func (ms *MsgStore) RemoveMsg(height uint64, round int64, step uint8, sender common.Address) {
	ms.Lock()
	defer ms.Unlock()
	delete(ms.messages[height][round][step], sender)
}

// Get take height and query conditions to query those msgs from msg store, it returns those msgs satisfied the condition.
func (ms *MsgStore) Get(height uint64, query func(*mUtils.Message) bool) []*mUtils.Message {
	ms.RLock()
	defer ms.RUnlock()

	var result []*mUtils.Message
	roundMap, ok := ms.messages[height]
	if !ok {
		return result
	}

	for _, msgTypeMap := range roundMap {
		for _, addressMap := range msgTypeMap {
			for _, msgs := range addressMap {
				for _, msg := range msgs {
					if query(msg) {
						result = append(result, msg)
					}
				}
			}
		}
	}

	return result
}
