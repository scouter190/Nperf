package auth

import "testing"

func TestSumVerify(t *testing.T) {
	mac := Sum("k", RoleClient, "sn", "cn")
	if !Verify("k", RoleClient, mac, "sn", "cn") {
		t.Fatal("MAC válido rechazado")
	}
	for _, bad := range [][4]string{{"x", RoleClient, "sn", "cn"}, {"k", RoleServer, "sn", "cn"},
		{"k", RoleClient, "s", "cn"}, {"k", RoleClient, "sn", "c"}} {
		if Verify(bad[0], bad[1], mac, bad[2], bad[3]) {
			t.Errorf("MAC aceptado con %v", bad)
		}
	}
	if Verify("k", RoleClient, "", "sn", "cn") {
		t.Error("MAC vacío aceptado")
	}
}

func TestNonceUnique(t *testing.T) {
	a, _ := NewNonce()
	b, _ := NewNonce()
	if a == b || len(a) != 32 {
		t.Fatalf("nonces %q %q", a, b)
	}
}
