package messges

import (
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/clearmatics/autonity/common"
	"github.com/clearmatics/autonity/core/types"
	"github.com/clearmatics/autonity/rlp"
)

type Validatable interface {
	Valid() bool
}

type NodeId interface {
}

type Proposal struct {
	Round      int64
	Height     *big.Int
	ValidRound int64
	NodeId     NodeId
	Value      Validatable
}

type Prevote struct {
	Round     int64
	Height    *big.Int
	NodeId    NodeId
	ValueHash []byte
}

type Precommit struct {
	Prevote
}

// RLP encoding doesn't support negative big.Int, so we have to pass one additionnal field to represents validRound = -1.
// Note that we could have as well indexed rounds starting by 1, but we want to stay close as possible to the spec.
func (p *Proposal) EncodeRLP(w io.Writer) error {
	if p.ProposalBlock == nil {
		// Should never happen
		return errors.New("encoderlp proposal with nil block")
	}

	isValidRoundNil := false
	var validRound uint64
	if p.ValidRound == -1 {
		validRound = 0
		isValidRoundNil = true
	} else {
		validRound = uint64(p.ValidRound)
	}

	return rlp.Encode(w, []interface{}{
		uint64(p.Round),
		p.Height,
		validRound,
		isValidRoundNil,
		p.ProposalBlock,
	})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (p *Proposal) DecodeRLP(s *rlp.Stream) error {
	var proposal struct {
		Round           uint64
		Height          *big.Int
		ValidRound      uint64
		IsValidRoundNil bool
		ProposalBlock   *types.Block
	}

	if err := s.Decode(&proposal); err != nil {
		return err
	}
	var validRound int64
	if proposal.IsValidRoundNil {
		if proposal.ValidRound != 0 {
			return errors.New("bad proposal validRound with isValidround nil")
		}
		validRound = -1
	} else {
		validRound = int64(proposal.ValidRound)
	}

	if !(validRound <= MaxRound && proposal.Round <= MaxRound) {
		return errors.New("bad proposal with invalid rounds")
	}

	if proposal.ProposalBlock == nil {
		return errors.New("bad proposal with nil decoded block")
	}

	p.Round = int64(proposal.Round)
	p.Height = proposal.Height
	p.ValidRound = validRound
	p.ProposalBlock = proposal.ProposalBlock

	return nil
}

// EncodeRLP serializes b into the Ethereum RLP format.
func (sub *Vote) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{uint64(sub.Round), sub.Height, sub.ProposedBlockHash})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (sub *Vote) DecodeRLP(s *rlp.Stream) error {
	var vote struct {
		Round             uint64
		Height            *big.Int
		ProposedBlockHash common.Hash
	}

	if err := s.Decode(&vote); err != nil {
		return err
	}
	sub.Round = int64(vote.Round)
	if sub.Round > MaxRound {
		return errInvalidMessage
	}
	sub.Height = vote.Height
	sub.ProposedBlockHash = vote.ProposedBlockHash
	return nil
}

func (sub *Vote) String() string {
	return fmt.Sprintf("{Round: %v, Height: %v ProposedBlockHash: %v}", sub.Round, sub.Height, sub.ProposedBlockHash.String())
}
