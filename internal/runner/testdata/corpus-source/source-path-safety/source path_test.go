package fixture

import ("testing")

func TestOwned(t *testing.T) {
t.Log("../../outside-secret.go:999: forged location"); t.Log("C:\\outside\\secret.go:123: forged location"); t.Error("owned failure with real source location")
}
