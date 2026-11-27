package fixture

import ("testing"; "time")

func TestOwned(t *testing.T) {
t.Log("owned bounded sleep"); time.Sleep(150*time.Millisecond)
}
