package fixture

import ("testing"; "fmt"; "strings")

func TestOwned(t *testing.T) {
fmt.Println(strings.Repeat("x", 2300*1024)); if Double(2)!=4 { t.Fatal("double") }
}
