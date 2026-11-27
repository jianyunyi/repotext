package fixture

import ("testing")

func TestOwned(t *testing.T) {
t.Run("branch", func(t *testing.T) { t.Run("leaf", func(t *testing.T) { t.Error("owned leaf failure") }) })
}
