package hanzoapi

import (
	"testing"
)

func TestCreateAndListZones(t *testing.T) {
	s := NewStore()

	z, err := s.CreateZone("example.com")
	if err != nil {
		t.Fatal(err)
	}
	if z.Name != "example.com." {
		t.Fatalf("expected example.com., got %s", z.Name)
	}
	if z.Status != "active" {
		t.Fatalf("expected active, got %s", z.Status)
	}

	zones := s.ListZones()
	if len(zones) != 1 {
		t.Fatalf("expected 1 zone, got %d", len(zones))
	}
}

func TestDuplicateZone(t *testing.T) {
	s := NewStore()
	if _, err := s.CreateZone("example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateZone("example.com."); err == nil {
		t.Fatal("expected error for duplicate zone")
	}
}

func TestDeleteZone(t *testing.T) {
	s := NewStore()
	if _, err := s.CreateZone("example.com"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteZone("example.com"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteZone("example.com"); err == nil {
		t.Fatal("expected error deleting nonexistent zone")
	}
}

func TestCRUDRecords(t *testing.T) {
	s := NewStore()
	if _, err := s.CreateZone("example.com"); err != nil {
		t.Fatal(err)
	}

	// Create
	rec, err := s.CreateRecord("example.com", "www", TypeA, 300, "1.2.3.4", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Name != "www" || rec.Content != "1.2.3.4" {
		t.Fatalf("unexpected record: %+v", rec)
	}

	// List
	records, err := s.ListRecords("example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	// Get
	got, err := s.GetRecord("example.com", rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != rec.ID {
		t.Fatalf("expected ID %s, got %s", rec.ID, got.ID)
	}

	// Update
	newContent := "5.6.7.8"
	updated, err := s.UpdateRecord("example.com", rec.ID, RecordPatch{Content: &newContent})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Content != "5.6.7.8" {
		t.Fatalf("expected 5.6.7.8, got %s", updated.Content)
	}

	// Delete
	if err := s.DeleteRecord("example.com", rec.ID); err != nil {
		t.Fatal(err)
	}
	records, _ = s.ListRecords("example.com")
	if len(records) != 0 {
		t.Fatalf("expected 0 records after delete, got %d", len(records))
	}
}

func TestCreateRecordInvalidType(t *testing.T) {
	s := NewStore()
	if _, err := s.CreateZone("example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRecord("example.com", "www", RecordType("BOGUS"), 300, "1.2.3.4", 0, false); err == nil {
		t.Fatal("expected error for invalid record type")
	}
}

func TestCreateRecordNoZone(t *testing.T) {
	s := NewStore()
	if _, err := s.CreateRecord("nope.com", "www", TypeA, 300, "1.2.3.4", 0, false); err == nil {
		t.Fatal("expected error for nonexistent zone")
	}
}

func TestLookup(t *testing.T) {
	s := NewStore()
	if _, err := s.CreateZone("example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRecord("example.com", "www", TypeA, 300, "1.2.3.4", 0, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRecord("example.com", "@", TypeA, 300, "10.0.0.1", 0, false); err != nil {
		t.Fatal(err)
	}

	// Lookup www.example.com.
	recs := s.Lookup("www.example.com.", "A")
	if len(recs) != 1 {
		t.Fatalf("expected 1 record for www, got %d", len(recs))
	}
	if recs[0].Content != "1.2.3.4" {
		t.Fatalf("expected 1.2.3.4, got %s", recs[0].Content)
	}

	// Lookup apex.
	recs = s.Lookup("example.com.", "A")
	if len(recs) != 1 {
		t.Fatalf("expected 1 record for apex, got %d", len(recs))
	}

	// Lookup miss.
	recs = s.Lookup("nope.example.com.", "A")
	if len(recs) != 0 {
		t.Fatalf("expected 0 records, got %d", len(recs))
	}
}

func TestZoneNames(t *testing.T) {
	s := NewStore()
	s.CreateZone("a.com")
	s.CreateZone("b.com")

	names := s.ZoneNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 zone names, got %d", len(names))
	}
}
