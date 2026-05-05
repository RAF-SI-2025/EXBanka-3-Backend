package models

import "testing"

func TestEmployee_PermissionNames(t *testing.T) {
	e := Employee{
		Permissions: []Permission{{Name: "employeeAdmin"}, {Name: "employeeAgent"}},
	}
	names := e.PermissionNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	if names[0] != "employeeAdmin" || names[1] != "employeeAgent" {
		t.Errorf("unexpected names: %v", names)
	}
}

func TestEmployee_PermissionNames_Empty(t *testing.T) {
	e := Employee{}
	names := e.PermissionNames()
	if len(names) != 0 {
		t.Errorf("expected 0 names, got %d", len(names))
	}
}

func TestClient_PermissionNames(t *testing.T) {
	c := Client{
		Permissions: []Permission{{Name: "clientBasic"}, {Name: "clientTrading"}},
	}
	names := c.PermissionNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	if names[1] != "clientTrading" {
		t.Errorf("unexpected names: %v", names)
	}
}

func TestEmployeeRoleLevel_Admin(t *testing.T) {
	if EmployeeRoleLevel(PermEmployeeAdmin) != 4 {
		t.Errorf("Admin level = %d, want 4", EmployeeRoleLevel(PermEmployeeAdmin))
	}
}

func TestEmployeeRoleLevel_Supervisor(t *testing.T) {
	if EmployeeRoleLevel(PermEmployeeSupervisor) != 3 {
		t.Errorf("Supervisor level = %d, want 3", EmployeeRoleLevel(PermEmployeeSupervisor))
	}
}

func TestEmployeeRoleLevel_Agent(t *testing.T) {
	if EmployeeRoleLevel(PermEmployeeAgent) != 2 {
		t.Errorf("Agent level = %d, want 2", EmployeeRoleLevel(PermEmployeeAgent))
	}
}

func TestEmployeeRoleLevel_Basic(t *testing.T) {
	if EmployeeRoleLevel(PermEmployeeBasic) != 1 {
		t.Errorf("Basic level = %d, want 1", EmployeeRoleLevel(PermEmployeeBasic))
	}
}

func TestEmployeeRoleLevel_Unknown(t *testing.T) {
	if EmployeeRoleLevel("unknown") != 0 {
		t.Errorf("Unknown level = %d, want 0", EmployeeRoleLevel("unknown"))
	}
}

func TestHasEmployeeRole_ExactMatch(t *testing.T) {
	if !HasEmployeeRole([]string{PermEmployeeAgent}, PermEmployeeAgent) {
		t.Error("expected true for exact match")
	}
}

func TestHasEmployeeRole_Higher(t *testing.T) {
	if !HasEmployeeRole([]string{PermEmployeeAdmin}, PermEmployeeBasic) {
		t.Error("expected true: admin should satisfy basic requirement")
	}
}

func TestHasEmployeeRole_Lower(t *testing.T) {
	if HasEmployeeRole([]string{PermEmployeeBasic}, PermEmployeeAdmin) {
		t.Error("expected false: basic should not satisfy admin requirement")
	}
}

func TestHasEmployeeRole_Empty(t *testing.T) {
	if HasEmployeeRole([]string{}, PermEmployeeBasic) {
		t.Error("expected false for empty permissions")
	}
}

func TestHasEmployeeRole_Multiple(t *testing.T) {
	if !HasEmployeeRole([]string{"unknown", PermEmployeeSupervisor}, PermEmployeeAgent) {
		t.Error("expected true: supervisor in list should satisfy agent requirement")
	}
}

func TestDefaultPermissions_HasAllRoles(t *testing.T) {
	got := make(map[string]bool)
	for _, p := range DefaultPermissions {
		got[p.Name] = true
	}
	expected := []string{PermEmployeeBasic, PermEmployeeAgent, PermEmployeeSupervisor, PermEmployeeAdmin, PermClientBasic, PermClientTrading}
	for _, e := range expected {
		if !got[e] {
			t.Errorf("DefaultPermissions missing %s", e)
		}
	}
}
