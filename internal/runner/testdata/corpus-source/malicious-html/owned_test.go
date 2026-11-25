package fixture

import ("testing")

func TestOwned(t *testing.T) {
t.Log(`<script>globalThis.fixtureAttack=true</script><img src=x onerror="alert(1)"><a href="javascript:alert(1)">owned text</a>`); if Double(2)!=4 { t.Fatal("double") }
}
