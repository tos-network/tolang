package lua

import "testing"

func TestMappingU256U256Semantics(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local m = mapping.new()
		assert(type(m) == "userdata")

		assert(m[1] == 0)
		assert(mapping.get(m, 1) == 0)
		assert(mapping.has(m, 1) == false)

		m[1] = 7
		assert(m[1] == 7)
		assert(mapping.get(m, 1) == 7)
		assert(mapping.has(m, 1) == true)

		mapping.set(m, 2, 9)
		assert(m[2] == 9)
		assert(mapping.get(m, 2) == 9)

		mapping.delete(m, 1)
		assert(m[1] == 0)
		assert(mapping.has(m, 1) == false)
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMappingAddressKeySemantics(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local m = mapping.new("address", "u256")
		local zero = "0x0000000000000000000000000000000000000000000000000000000000000000"
		local a = "0x1111111111111111111111111111111111111111111111111111111111111111"
		local upper = "0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
		local lower = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

		assert(m[zero] == 0)
		assert(mapping.key_type(m) == "address")
		assert(mapping.val_type(m) == "u256")

		m[a] = 5
		assert(m[a] == 5)
		assert(mapping.has(m, a))

		m[upper] = 3
		assert(m[lower] == 3)

		mapping.delete(m, a)
		assert(m[a] == 0)
		assert(mapping.has(m, a) == false)
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMappingRejectsInvalidAccessAndEnumeration(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`
		local m = mapping.new("u256", "bool")
		assert(m[1] == false)

		local ok

		ok = pcall(function() m["abc"] = true end)
		assert(ok == false)

		ok = pcall(function() m[1] = 2 end)
		assert(ok == false)

		ok = pcall(function() mapping.new("badtype", "u256") end)
		assert(ok == false)

		ok = pcall(function()
			for k, v in pairs(m) do
			end
		end)
		assert(ok == false)
	`)
	if err != nil {
		t.Fatal(err)
	}
}
