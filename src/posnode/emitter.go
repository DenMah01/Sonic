package posnode

import (
	"sort"
	"sync"
	"time"

	"github.com/Fantom-foundation/go-lachesis/src/hash"
	"github.com/Fantom-foundation/go-lachesis/src/inter"
)

// emitter creates events from external transactions.
type emitter struct {
	transactions [][]byte
	done         chan struct{}

	sync.Mutex
}

// StartEventEmission starts event emission.
func (n *Node) StartEventEmission() {
	if n.emitter.done != nil {
		return
	}
	n.emitter.done = make(chan struct{})

	go func() {
		ticker := time.NewTicker(n.conf.EmitInterval)
		for {
			select {
			case <-ticker.C:
				n.EmitEvent()
			case <-n.emitter.done:
				return
			}
		}
	}()
}

// StopEventEmission stops event emission.
func (n *Node) StopEventEmission() {
	close(n.emitter.done)
	n.emitter.done = nil
}

// AddTransaction adds transaction into the node.
func (n *Node) AddTransaction(t []byte) {
	n.emitter.Lock()
	defer n.emitter.Unlock()

	n.emitter.transactions = append(n.emitter.transactions, t)
}

// EmitEvent takes all transactions from buffer builds event,
// connects it with given amount of parents, sign and put it into the storage.
// It returns emmited event for test purpose.
func (n *Node) EmitEvent() *inter.Event {
	n.emitter.Lock()
	defer n.emitter.Unlock()

	var (
		index        uint64
		parents      hash.Events = hash.Events{}
		lamportTime  inter.Timestamp
		transactions [][]byte
	)

	// transactions buffer swap
	transactions, n.emitter.transactions = n.emitter.transactions, nil

	// ref nodes selection
	refs := n.peers.Snapshot()
	sort.Sort(n.emitterEvaluation(refs))
	count := n.conf.EventParentsCount - 1
	if len(refs) > count {
		refs = refs[:count]
	}
	refs = append(refs, n.ID)

	// last events of ref nodes
	for _, ref := range refs {
		h := n.store.GetPeerHeight(ref)
		if h < 1 {
			if ref == n.ID {
				index = 1
				parents.Add(hash.ZeroEvent)
			}
			continue
		}

		if ref == n.ID {
			index = h + 1
		}

		e := n.store.GetEventHash(ref, h)
		if e == nil {
			n.log.Errorf("no event hash for (%s,%d) in store", ref.String(), h)
			continue
		}
		event := n.store.GetEvent(*e)
		if event == nil {
			n.log.Errorf("no event %s in store", e.String())
			continue
		}

		parents.Add(*e)
		if lamportTime < event.LamportTime {
			lamportTime = event.LamportTime
		}
	}

	event := &inter.Event{
		Index:                index,
		Creator:              n.ID,
		Parents:              parents,
		LamportTime:          lamportTime + 1,
		ExternalTransactions: transactions,
	}
	if err := event.SignBy(n.key); err != nil {
		panic(err)
	}

	n.saveNewEvent(event)

	return event
}

/*
 * evaluation function for emitter
 */

func (n *Node) emitterEvaluation(peers []hash.Peer) *emitterEvaluation {
	return &emitterEvaluation{
		node:  n,
		peers: peers,
	}
}

// emitterEvaluation implements sort.Interface.
type emitterEvaluation struct {
	node  *Node
	peers []hash.Peer
}

// Len is the number of elements in the collection.
func (n *emitterEvaluation) Len() int {
	return len(n.peers)
}

// Swap swaps the elements with indexes i and j.
func (n *emitterEvaluation) Swap(i, j int) {
	n.peers[i], n.peers[j] = n.peers[j], n.peers[i]
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (n *emitterEvaluation) Less(i, j int) bool {
	// TODO: implement it
	return false
}
