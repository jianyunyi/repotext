package fixture

import ("testing")

func TestOwned(t *testing.T) {
t.Log("中文日志 café 😀 <>&"); if Double(2)!=4 { t.Fatal("double") }
}
