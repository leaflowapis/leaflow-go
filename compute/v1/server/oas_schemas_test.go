package computev1server

import (
	"encoding/json"
	"testing"

	"github.com/go-faster/jx"
)

func TestNativeCreateElasticIPIPv4AndProduct(t *testing.T) {
	for _, scenario := range []struct {
		address string
		valid   bool
	}{
		{"0.0.0.0", true},
		{"192.0.2.1", true},
		{"9.10.99.100", true},
		{"199.200.249.250", true},
		{"255.255.255.255", true},
		{"2001:db8::10", false},
		{"::ffff:192.0.2.10", false},
		{"192.0.2.10/32", false},
		{"256.0.0.1", false},
		{"192.00.2.10", false},
		{"01.2.3.4", false},
		{"192.0.2", false},
		{"", false},
	} {
		t.Run(scenario.address, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{
				"type_id":                  "eiptype_00000000000000000000000001",
				"bandwidth_mbps":           50,
				"billing":                  map[string]string{"mode": "postpaid"},
				"address":                  scenario.address,
			})
			if err != nil {
				t.Fatal(err)
			}
			var creation CreateElasticIPRequestBody
			err = creation.Decode(jx.DecodeBytes(body))
			if err == nil {
				err = creation.Validate()
			}
			if (err == nil) != scenario.valid {
				t.Fatalf("valid=%v, request decode/validate=%v", scenario.valid, err)
			}
			creation = CreateElasticIPRequestBody{
				Address:               NewOptIPv4HostAddress(IPv4HostAddress(scenario.address)),
				TypeID:                "eiptype_00000000000000000000000001",
				BandwidthMbps:         50,
				Billing:               BillingChoice{Mode: BillingChoiceModePostpaid},
			}
			if err := creation.Validate(); (err == nil) != scenario.valid {
				t.Fatalf("valid=%v, constructed request Validate=%v", scenario.valid, err)
			}
		})
	}
	for _, scenario := range []struct {
		id    ElasticIPTypeID
		valid bool
	}{
		{"eiptype_00000000000000000000000001", true},
		{"11111111-1111-1111-1111-111111111111", false},
		{"eip_00000000000000000000000001", false},
		{"eiptype_80000000000000000000000001", false},
	} {
		t.Run(string(scenario.id), func(t *testing.T) {
			if err := scenario.id.Validate(); (err == nil) != scenario.valid {
				t.Fatalf("valid=%v, Validate=%v", scenario.valid, err)
			}
		})
	}
}

func TestNativeResizeMachineTypeIdentity(t *testing.T) {
	for _, scenario := range []struct {
		name, body string
		valid      bool
	}{
		{"typed", `{"machine_type_id":"mtype_00000000000000000000000001"}`, true},
		{"UUID", `{"machine_type_id":"11111111-1111-1111-1111-111111111111"}`, false},
		{"wrong prefix", `{"machine_type_id":"vm_00000000000000000000000001"}`, false},
		{"overflow", `{"machine_type_id":"mtype_80000000000000000000000001"}`, false},
		{"old field", `{"virtual_machine_type_id":"mtype_00000000000000000000000001"}`, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var request ResizeVirtualMachineRequestBody
			err := request.Decode(jx.DecodeBytes([]byte(scenario.body)))
			if err == nil {
				err = request.Validate()
			}
			if (err == nil) != scenario.valid {
				t.Fatalf("valid=%v, decode/validate=%v", scenario.valid, err)
			}
			if scenario.valid {
				var identity MachineTypeID = request.MachineTypeID
				if err := identity.Validate(); err != nil {
					t.Fatal(err)
				}
				wire, err := json.Marshal(&request)
				if err != nil {
					t.Fatal(err)
				}
				if string(wire) != scenario.body {
					t.Fatalf("identity wire shape changed: %s", wire)
				}
			}
		})
	}
}

func TestNativeRegionResourceUsesRegionIdentity(t *testing.T) {
	for _, scenario := range []struct {
		identity string
		valid    bool
	}{
		{"region_00000000000000000000000001", true},
		{"11111111-1111-1111-1111-111111111111", false},
		{"eiptype_00000000000000000000000001", false},
	} {
		body := `{"id":"` + scenario.identity + `","code":"hk-1","name":"Hong Kong","country_code":"HK"}`
		var region RegionResource
		err := region.Decode(jx.DecodeBytes([]byte(body)))
		if err == nil {
			err = region.Validate()
		}
		if (err == nil) != scenario.valid {
			t.Fatalf("valid=%v, region decode/validate=%v", scenario.valid, err)
		}
		if scenario.valid {
			var identity RegionID = region.ID
			if err := identity.Validate(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestNativeAllResourceTypeIDContracts(t *testing.T) {
	for _, kind := range []struct {
		name, prefix   string
		decodeValidate func([]byte) error
	}{
		{"VirtualMachineID", "vm", func(body []byte) error {
			var identity VirtualMachineID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"NetworkInterfaceID", "eni", func(body []byte) error {
			var identity NetworkInterfaceID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"VirtualPrivateCloudID", "vpc", func(body []byte) error {
			var identity VirtualPrivateCloudID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"ElasticIPID", "eip", func(body []byte) error {
			var identity ElasticIPID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"ElasticIPTypeID", "eiptype", func(body []byte) error {
			var identity ElasticIPTypeID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"RegionID", "region", func(body []byte) error {
			var identity RegionID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"MachineTypeID", "mtype", func(body []byte) error {
			var identity MachineTypeID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"AvailabilityZoneID", "zone", func(body []byte) error {
			var identity AvailabilityZoneID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"DiskID", "disk", func(body []byte) error {
			var identity DiskID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"DiskBackupID", "backup", func(body []byte) error {
			var identity DiskBackupID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"DiskTypeID", "disktype", func(body []byte) error {
			var identity DiskTypeID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"SnapshotID", "snapshot", func(body []byte) error {
			var identity SnapshotID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"VirtualMachineDiskID", "vmdisk", func(body []byte) error {
			var identity VirtualMachineDiskID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"NetworkInterfaceAddressID", "eniaddr", func(body []byte) error {
			var identity NetworkInterfaceAddressID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"ElasticIPAssociationID", "eipassoc", func(body []byte) error {
			var identity ElasticIPAssociationID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"ImageID", "image", func(body []byte) error {
			var identity ImageID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"SecurityGroupID", "sg", func(body []byte) error {
			var identity SecurityGroupID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"SubnetID", "subnet", func(body []byte) error {
			var identity SubnetID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"VirtualMachineNetworkInterfaceID", "vmeni", func(body []byte) error {
			var identity VirtualMachineNetworkInterfaceID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"OperationLogID", "oplog", func(body []byte) error {
			var identity OperationLogID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"PeeringID", "peering", func(body []byte) error {
			var identity PeeringID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"RouteID", "route", func(body []byte) error {
			var identity RouteID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"SecurityGroupRuleID", "sgrule", func(body []byte) error {
			var identity SecurityGroupRuleID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
		{"VirtualMachineRestrictionID", "vmrestriction", func(body []byte) error {
			var identity VirtualMachineRestrictionID
			if err := identity.Decode(jx.DecodeBytes(body)); err != nil {
				return err
			}
			return identity.Validate()
		}},
	} {
		t.Run(kind.name, func(t *testing.T) {
			for _, scenario := range []struct {
				value string
				valid bool
			}{
				{kind.prefix + "_00000000000000000000000001", true},
				{"11111111-1111-1111-1111-111111111111", false},
				{"wrong_00000000000000000000000001", false},
				{kind.prefix + "_80000000000000000000000001", false},
				{kind.prefix + "_0000000000000000000000000i", false},
			} {
				body, err := json.Marshal(scenario.value)
				if err != nil {
					t.Fatal(err)
				}
				if err := kind.decodeValidate(body); (err == nil) != scenario.valid {
					t.Fatalf("valid=%v, decode/validate=%v", scenario.valid, err)
				}
			}
		})
	}
}
