package reconcile

import (
	"testing"

	"github.com/GermanRaulGarcia/invoicesup_agent/internal/api"
	"github.com/GermanRaulGarcia/invoicesup_agent/internal/state"
)

func exists(codes ...string) func(string) bool {
	set := map[string]bool{}
	for _, c := range codes {
		set[c] = true
	}
	return func(code string) bool { return set[code] }
}

func batch(code, token string) api.Batch {
	return api.Batch{BusinessCode: code, Filename: code + "_facturas.txt", Content: "R#\r\n", BatchToken: token}
}

// (a) idle business with pending content and no local file → write.
func TestIdlePendingWrites(t *testing.T) {
	actions := Reconcile([]api.Batch{batch("SPM", "t1")}, exists(), state.Store{})
	if len(actions) != 1 || actions[0].Kind != Write || actions[0].Code != "SPM" || actions[0].Token != "t1" {
		t.Fatalf("expected one write for SPM, got %+v", actions)
	}
}

// (b) file still present (Golden has not imported) → no action.
func TestFilePresentNoAction(t *testing.T) {
	store := state.Store{"SPM": {Token: "t1", State: state.Written}}
	if actions := Reconcile([]api.Batch{batch("SPM", "t1")}, exists("SPM"), store); len(actions) != 0 {
		t.Fatalf("expected no action while file present, got %+v", actions)
	}
}

// (c) written + file gone → confirm the persisted token.
func TestWrittenFileGoneConfirms(t *testing.T) {
	store := state.Store{"SPM": {Token: "t1", State: state.Written}}
	actions := Reconcile(nil, exists(), store)
	if len(actions) != 1 || actions[0].Kind != Confirm || actions[0].Token != "t1" {
		t.Fatalf("expected confirm t1, got %+v", actions)
	}
}

// (d) awaiting_confirm → confirm retry.
func TestAwaitingConfirmRetries(t *testing.T) {
	store := state.Store{"SPM": {Token: "t1", State: state.AwaitingConfirm}}
	actions := Reconcile(nil, exists(), store)
	if len(actions) != 1 || actions[0].Kind != Confirm || actions[0].Token != "t1" {
		t.Fatalf("expected confirm retry t1, got %+v", actions)
	}
}

// (e) mid-cycle code with new pending (superset token) → no second write, and
// confirm still targets the ORIGINAL persisted token, not the new one.
func TestMidCycleDoesNotDoubleWrite(t *testing.T) {
	store := state.Store{"SPM": {Token: "t1", State: state.Written}}
	// New pending batch t2 for SPM while its file still sits on disk.
	actions := Reconcile([]api.Batch{batch("SPM", "t2")}, exists("SPM"), store)
	if len(actions) != 0 {
		t.Fatalf("expected no action mid-cycle, got %+v", actions)
	}
}

// (e') mid-cycle, file now gone → confirm the ORIGINAL token t1, not pending t2.
func TestMidCycleConfirmsOriginalToken(t *testing.T) {
	store := state.Store{"SPM": {Token: "t1", State: state.Written}}
	actions := Reconcile([]api.Batch{batch("SPM", "t2")}, exists(), store)
	if len(actions) != 1 || actions[0].Kind != Confirm || actions[0].Token != "t1" {
		t.Fatalf("expected confirm original t1, got %+v", actions)
	}
}

// (f) orphan adoption: a file present with no entry is adopted as written using
// its pending batch's token, and is then NOT rewritten.
func TestAdoptOrphanThenNoWrite(t *testing.T) {
	store := AdoptOrphans([]api.Batch{batch("SPM", "t1")}, exists("SPM"), state.Store{})
	if store["SPM"].State != state.Written || store["SPM"].Token != "t1" {
		t.Fatalf("orphan not adopted as written: %+v", store)
	}
	if actions := Reconcile([]api.Batch{batch("SPM", "t1")}, exists("SPM"), store); len(actions) != 0 {
		t.Fatalf("adopted orphan should not be rewritten, got %+v", actions)
	}
}

// A lingering file with no matching pending batch is left alone (already
// delivered server-side).
func TestAdoptIgnoresFileWithoutPendingBatch(t *testing.T) {
	store := AdoptOrphans(nil, exists("SPM"), state.Store{})
	if len(store) != 0 {
		t.Fatalf("expected no adoption without a pending batch, got %+v", store)
	}
}

// Two businesses, independent lifecycles in one tick.
func TestMultipleBusinessesIndependent(t *testing.T) {
	store := state.Store{"AAA": {Token: "ta", State: state.Written}} // file gone → confirm
	pending := []api.Batch{batch("AAA", "ta"), batch("BBB", "tb")}   // BBB idle → write
	actions := Reconcile(pending, exists(), store)
	var confirms, writes int
	for _, a := range actions {
		switch a.Kind {
		case Confirm:
			if a.Code != "AAA" || a.Token != "ta" {
				t.Errorf("unexpected confirm %+v", a)
			}
			confirms++
		case Write:
			if a.Code != "BBB" || a.Token != "tb" {
				t.Errorf("unexpected write %+v", a)
			}
			writes++
		}
	}
	if confirms != 1 || writes != 1 {
		t.Fatalf("expected 1 confirm + 1 write, got %d/%d (%+v)", confirms, writes, actions)
	}
}
