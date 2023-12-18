package atc

import (
	"github.com/autonity/autonity/core"
)

type CommitteeWatcher struct {
	newEpoch chan core.ChainHeadEvent
	chain    *core.BlockChain
}

func NewCommitteeWatcher(chain *core.BlockChain) *CommitteeWatcher {
	return &CommitteeWatcher{chain: chain}

}

func (c *CommitteeWatcher) Run() {
	var newHead = make(chan core.ChainHeadEvent, 10)
	sub := c.chain.SubscribeChainHeadEvent(newHead)

	go func() {
		defer sub.Unsubscribe()
		for {
			select {
			case <-c.newEpoch:
				//tODO
			case <-sub.Err():
				// Would be nice to sync with Stop, but there is no
				// good way to do that.
				return
			}
		}
	}()
}

func Stop() {

}
