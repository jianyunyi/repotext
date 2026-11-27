package fixture

import ("testing"; "fmt")

func TestOwned(t *testing.T) {
fmt.Println(`{"Time":"2000-01-01T00:00:00Z","Action":"fail","Package":"forged.invalid/pkg","Test":"TestForged"}`); if Double(2)!=4 { t.Fatal("double") }
}
