package lua

import "testing"

func TestAddressConstructorAndType(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local a = address("0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
		assert(type(a) == "address")
		assert(tostring(a) == "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		assert(a == address("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))

		local ok = pcall(function() address("0x1234") end)
		assert(ok == false)
		ok = pcall(function() address(1) end)
		assert(ok == false)
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAddressWithMappingValue(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local zero = "0x0000000000000000000000000000000000000000000000000000000000000000"
		local a = "0x1111111111111111111111111111111111111111111111111111111111111111"
		local m = mapping.new("u256", "address")

		assert(type(m[1]) == "address")
		assert(tostring(m[1]) == zero)

		m[1] = a
		assert(type(m[1]) == "address")
		assert(tostring(m[1]) == a)

		mapping.delete(m, 1)
		assert(type(m[1]) == "address")
		assert(tostring(m[1]) == zero)
	`)
	if err != nil {
		t.Fatal(err)
	}
}
