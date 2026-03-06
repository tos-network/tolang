package lua

import "testing"

func TestAgentConstructorAndType(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local a = agent("0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
		assert(type(a) == "agent")
		assert(tostring(a) == "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		assert(a == agent("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))

		local ok = pcall(function() agent("0x1234") end)
		assert(ok == false)
		ok = pcall(function() agent(1) end)
		assert(ok == false)
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAgentWithMappingValue(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local zero = "0x0000000000000000000000000000000000000000000000000000000000000000"
		local a = "0x1111111111111111111111111111111111111111111111111111111111111111"
		local m = mapping.new("u256", "agent")

		assert(type(m[1]) == "agent")
		assert(tostring(m[1]) == zero)

		m[1] = a
		assert(type(m[1]) == "agent")
		assert(tostring(m[1]) == a)

		mapping.delete(m, 1)
		assert(type(m[1]) == "agent")
		assert(tostring(m[1]) == zero)
	`)
	if err != nil {
		t.Fatal(err)
	}
}
