package fixture

import ("testing")

func TestOwned(t *testing.T) {
if Double(2) != 5 { t.Errorf("expected 5; got %d", Double(2)) }
}
