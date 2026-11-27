package fixture

import ("testing"; "time")

func TestOwned(t *testing.T) {
for _, name := range []string{"left", "right"} { t.Run(name, func(t *testing.T) { t.Parallel(); t.Log("parallel start"); time.Sleep(5*time.Millisecond); if Double(2)!=4 { t.Fatal("double") }; t.Log("parallel finish") }) }
}
