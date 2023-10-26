package accountability

import (
	"errors"
	"github.com/autonity/autonity/autonity"
	"github.com/autonity/autonity/consensus/tendermint/core/message"
	"github.com/autonity/autonity/rlp"
	"io"
)

var (
	errUnexpectedCode = errors.New("unexpected message code")
)

type typedMessage struct {
	message.Message
}

func (t *typedMessage) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []any{t.Code(), t.Message})
}

func (t *typedMessage) DecodeRLP(stream *rlp.Stream) error {
	if _, err := stream.List(); err != nil {
		return err
	}
	b, err := stream.Bytes()
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return rlp.ErrExpectedString
	}
	var p message.Message
	switch b[0] {
	case message.PrevoteCode:
		p = &message.Prevote{}
	case message.PrecommitCode:
		p = &message.Precommit{}
	case message.LightProposalCode:
		p = &message.LightProposal{}
	default:
		return errUnexpectedCode
	}
	if err := stream.Decode(p); err != nil {
		return err
	}
	t.Message = p
	return stream.ListEnd()
}

// Proof is what to prove that one is misbehaving, one should be slashed when a valid Proof is rise.
type Proof struct {
	Type      autonity.AccountabilityEventType // Accountability event types: Misbehaviour, Accusation, Innocence.
	Rule      autonity.Rule                    // Rule ID defined in AFD rule engine.
	Message   message.Message                  // the consensus message which is accountable.
	Evidences []message.Message                // the proofs of the accountability event.
}

type encodedProof struct {
	Type      autonity.AccountabilityEventType
	Rule      autonity.Rule
	Message   typedMessage
	Evidences []typedMessage
}

func (p *Proof) EncodeRLP(w io.Writer) error {
	encoded := encodedProof{
		Type: p.Type,
		Rule: p.Rule,
	}
	encoded.Message = typedMessage{p.Message}
	for _, m := range p.Evidences {
		encoded.Evidences = append(encoded.Evidences, typedMessage{m})
	}
	return rlp.Encode(w, &encoded)
}

func (p *Proof) DecodeRLP(stream *rlp.Stream) error {
	encoded := encodedProof{}
	if err := stream.Decode(&encoded); err != nil {
		return err
	}
	p.Type = encoded.Type
	p.Rule = encoded.Rule
	p.Message = encoded.Message
	p.Evidences = make([]message.Message, len(encoded.Evidences))
	for i := range encoded.Evidences {
		p.Evidences[i] = encoded.Evidences[i]
	}
	return nil
}
