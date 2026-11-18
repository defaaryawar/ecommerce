package outbox

import "testing"

func headerMap(hs []struct{ k, v string }) map[string]string {
	m := map[string]string{}
	for _, h := range hs {
		m[h.k] = h.v
	}
	return m
}

func TestToRecord_DenganTenant(t *testing.T) {
	tid := "tenant-1"
	rec := toRecord(row{
		ID: "evt-1", Topic: "identity.tenant.events.v1", AggregateID: "agg-1",
		EventType: "identity.tenant.registered.v1", TenantID: &tid, Payload: `{"slug":"toko-budi"}`,
	})

	if rec.Topic != "identity.tenant.events.v1" {
		t.Fatalf("topic salah: %s", rec.Topic)
	}
	if string(rec.Key) != "tenant-1" {
		t.Fatalf("key harus tenant_id, dapat %q", rec.Key)
	}
	if string(rec.Value) != `{"slug":"toko-budi"}` {
		t.Fatalf("value harus payload mentah, dapat %q", rec.Value)
	}

	var hs []struct{ k, v string }
	for _, h := range rec.Headers {
		hs = append(hs, struct{ k, v string }{h.Key, string(h.Value)})
	}
	got := headerMap(hs)
	if got["event_id"] != "evt-1" || got["event_type"] != "identity.tenant.registered.v1" || got["tenant_id"] != "tenant-1" {
		t.Fatalf("header salah: %v", got)
	}
}

func TestToRecord_TanpaTenant_KeyPakaiAggregateID(t *testing.T) {
	rec := toRecord(row{ID: "evt-2", Topic: "x.events.v1", AggregateID: "agg-9", EventType: "x.y.v1", Payload: `{}`})

	if string(rec.Key) != "agg-9" {
		t.Fatalf("tanpa tenant, key harus aggregate_id, dapat %q", rec.Key)
	}
	for _, h := range rec.Headers {
		if h.Key == "tenant_id" {
			t.Fatal("tanpa tenant, header tenant_id tidak boleh ada")
		}
	}
}
