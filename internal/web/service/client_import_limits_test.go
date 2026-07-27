package service

import "testing"

func TestClientImportRejectsTooManyRows(t *testing.T) {
	items := make([]ClientCreatePayload, MaxClientImportRows+1)
	svc := &ClientService{}

	if _, _, err := svc.BulkCreate(nil, items); err == nil {
		t.Fatal("BulkCreate accepted an oversized import")
	}
	if _, _, err := svc.ImportClients(nil, items); err == nil {
		t.Fatal("ImportClients accepted an oversized import")
	}
}
