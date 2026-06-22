package main

import (
	"strings"
	"testing"

	"github.com/hyperledger/fabric-chaincode-go/pkg/cid"
	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-contract-api-go/contractapi"
	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
)

// These tests pin F14's guarantee: Merkle-batch state is partitioned by the
// caller's *identity-derived* tenant. A caller cannot read, overwrite, or
// enumerate another tenant's batches, and the tenant cannot be forged through a
// function argument (only the signed identity governs the scope).

// --- fake client identity -------------------------------------------------

type fakeCID struct {
	cid.ClientIdentity
	mspID string
	attrs map[string]string
}

func (f *fakeCID) GetMSPID() (string, error) { return f.mspID, nil }
func (f *fakeCID) GetAttributeValue(name string) (string, bool, error) {
	v, ok := f.attrs[name]
	return v, ok, nil
}

// --- fake stub (in-memory world state with composite keys) ----------------

const ckSep = "\x00"

type fakeStub struct {
	shim.ChaincodeStubInterface
	state map[string][]byte
}

func newFakeStub() *fakeStub { return &fakeStub{state: map[string][]byte{}} }

func (s *fakeStub) GetState(k string) ([]byte, error) { return s.state[k], nil }
func (s *fakeStub) PutState(k string, v []byte) error { s.state[k] = v; return nil }
func (s *fakeStub) DelState(k string) error           { delete(s.state, k); return nil }

func (s *fakeStub) CreateCompositeKey(objectType string, attrs []string) (string, error) {
	return ckSep + objectType + ckSep + strings.Join(attrs, ckSep) + ckSep, nil
}

func (s *fakeStub) GetStateByPartialCompositeKey(objectType string, attrs []string) (shim.StateQueryIteratorInterface, error) {
	prefix := ckSep + objectType + ckSep
	if len(attrs) > 0 {
		prefix += strings.Join(attrs, ckSep) + ckSep
	}
	it := &fakeIterator{}
	for k, v := range s.state {
		if strings.HasPrefix(k, prefix) {
			it.items = append(it.items, queryresult.KV{Key: k, Value: v})
		}
	}
	return it, nil
}

type fakeIterator struct {
	items []queryresult.KV
	i     int
}

func (it *fakeIterator) HasNext() bool { return it.i < len(it.items) }
func (it *fakeIterator) Next() (*queryresult.KV, error) {
	kv := it.items[it.i]
	it.i++
	return &kv, nil
}
func (it *fakeIterator) Close() error { return nil }

// --- fake transaction context --------------------------------------------

type fakeCtx struct {
	contractapi.TransactionContextInterface
	stub shim.ChaincodeStubInterface
	id   cid.ClientIdentity
}

func (c *fakeCtx) GetStub() shim.ChaincodeStubInterface  { return c.stub }
func (c *fakeCtx) GetClientIdentity() cid.ClientIdentity { return c.id }

// ctxFor builds a context whose identity carries the given tenant attribute.
func ctxFor(stub shim.ChaincodeStubInterface, tenant string) *fakeCtx {
	return &fakeCtx{stub: stub, id: &fakeCID{mspID: "Org1MSP", attrs: map[string]string{"tenant": tenant}}}
}

// ctxForMSP builds a context with no tenant attribute, isolating by MSP ID.
func ctxForMSP(stub shim.ChaincodeStubInterface, mspID string) *fakeCtx {
	return &fakeCtx{stub: stub, id: &fakeCID{mspID: mspID, attrs: map[string]string{}}}
}

// --- tests ----------------------------------------------------------------

func TestStoreAndQueryAreScopedToTenant(t *testing.T) {
	sc := new(SmartContract)
	stub := newFakeStub()

	if err := sc.StoreMerkleRoot(ctxFor(stub, "acme"), "b1", "rootA", "", 1, `["h1"]`); err != nil {
		t.Fatalf("acme store: %v", err)
	}
	// Owner reads its batch back.
	got, err := sc.QueryMerkleBatch(ctxFor(stub, "acme"), "b1")
	if err != nil || got.MerkleRoot != "rootA" {
		t.Fatalf("acme query b1 = %+v, %v; want rootA", got, err)
	}
	// A different tenant cannot read it (no cross-tenant leak).
	if _, err := sc.QueryMerkleBatch(ctxFor(stub, "globex"), "b1"); err == nil {
		t.Fatal("globex read of acme's batch b1: want error, got nil")
	}
}

func TestCrossTenantCannotOverwrite(t *testing.T) {
	sc := new(SmartContract)
	stub := newFakeStub()

	if err := sc.StoreMerkleRoot(ctxFor(stub, "acme"), "b1", "rootA", "", 1, `["h1"]`); err != nil {
		t.Fatalf("acme store: %v", err)
	}
	// globex storing the same batch id writes to its own partition; it must not
	// touch acme's anchor.
	if err := sc.StoreMerkleRoot(ctxFor(stub, "globex"), "b1", "rootB", "", 1, `["h2"]`); err != nil {
		t.Fatalf("globex store: %v", err)
	}
	got, err := sc.QueryMerkleBatch(ctxFor(stub, "acme"), "b1")
	if err != nil || got.MerkleRoot != "rootA" {
		t.Fatalf("acme b1 after globex store = %+v, %v; want rootA intact", got, err)
	}
	// Write-once still holds within a tenant.
	if err := sc.StoreMerkleRoot(ctxFor(stub, "acme"), "b1", "rootA2", "", 1, `["h1"]`); err == nil {
		t.Fatal("re-anchoring acme/b1: want write-once error, got nil")
	}
}

func TestGetAllReturnsOnlyCallerTenant(t *testing.T) {
	sc := new(SmartContract)
	stub := newFakeStub()

	_ = sc.StoreMerkleRoot(ctxFor(stub, "acme"), "b1", "r1", "", 1, `["h"]`)
	_ = sc.StoreMerkleRoot(ctxFor(stub, "acme"), "b2", "r2", "", 1, `["h"]`)
	_ = sc.StoreMerkleRoot(ctxFor(stub, "globex"), "b3", "r3", "", 1, `["h"]`)

	acme, err := sc.GetAllMerkleBatches(ctxFor(stub, "acme"))
	if err != nil {
		t.Fatalf("acme GetAll: %v", err)
	}
	if len(acme) != 2 {
		t.Fatalf("acme GetAll returned %d batches, want 2", len(acme))
	}
	for _, b := range acme {
		if b.BatchID == "b3" {
			t.Fatal("acme GetAll leaked globex's batch b3")
		}
	}
	globex, _ := sc.GetAllMerkleBatches(ctxFor(stub, "globex"))
	if len(globex) != 1 || globex[0].BatchID != "b3" {
		t.Fatalf("globex GetAll = %+v, want only b3", globex)
	}
}

func TestTenantNotSpoofableViaBatchID(t *testing.T) {
	sc := new(SmartContract)
	stub := newFakeStub()

	// acme anchors a batch whose id *looks* like it belongs to globex. Scoping
	// must follow the identity (acme), not the argument string.
	if err := sc.StoreMerkleRoot(ctxFor(stub, "acme"), "globex-domain-deadbeef", "rootA", "", 1, `["h"]`); err != nil {
		t.Fatalf("store: %v", err)
	}
	if _, err := sc.QueryMerkleBatch(ctxFor(stub, "globex"), "globex-domain-deadbeef"); err == nil {
		t.Fatal("globex read of an acme batch with a globex-looking id: want error, got nil")
	}
}

func TestTenantFallsBackToMSPWhenNoAttribute(t *testing.T) {
	sc := new(SmartContract)
	stub := newFakeStub()

	if err := sc.StoreMerkleRoot(ctxForMSP(stub, "Org2MSP"), "b1", "rootO2", "", 1, `["h"]`); err != nil {
		t.Fatalf("Org2 store: %v", err)
	}
	if _, err := sc.QueryMerkleBatch(ctxForMSP(stub, "Org3MSP"), "b1"); err == nil {
		t.Fatal("Org3 read of Org2's batch: want error, got nil")
	}
	got, err := sc.QueryMerkleBatch(ctxForMSP(stub, "Org2MSP"), "b1")
	if err != nil || got.MerkleRoot != "rootO2" {
		t.Fatalf("Org2 query = %+v, %v; want rootO2", got, err)
	}
}
