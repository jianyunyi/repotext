package fixture

import ("testing")

func TestOwned(t *testing.T) {
t.Log("first line\nsecond line\nthird line"); if Double(2)!=4 { t.Fatal("double") }
}
