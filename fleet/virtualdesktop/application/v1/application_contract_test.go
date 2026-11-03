package applicationv1

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestOpenServiceDescriptor(t *testing.T) {
	service := File_leaflow_fleet_virtualdesktop_application_v1_application_proto.Services().ByName("ApplicationService")
	if service == nil {
		t.Fatal("ApplicationService is missing")
	}
	method := service.Methods().ByName("Open")
	if method == nil {
		t.Fatal("ApplicationService.Open is missing")
	}
	if got, want := method.Input().FullName(), (&OpenRequest{}).ProtoReflect().Descriptor().FullName(); got != want {
		t.Fatalf("Open input = %s, want %s", got, want)
	}
	if got, want := method.Output().FullName(), (&OpenResponse{}).ProtoReflect().Descriptor().FullName(); got != want {
		t.Fatalf("Open output = %s, want %s", got, want)
	}
}

func TestOpenRequestRoundTrip(t *testing.T) {
	for _, request := range []*OpenRequest{
		{Target: "/usr/bin/example", Arguments: []string{"--literal", "$HOME", "a b"}},
		{
			Target:               "example.desktop",
			ActionId:             "new-window",
			Uris:                 []string{"file:///tmp/report.txt", "https://example.com/"},
			LauncherConfirmation: LauncherConfirmation_LAUNCHER_CONFIRMATION_LAUNCH_ANYWAY,
		},
	} {
		t.Run(request.Target, func(t *testing.T) {
			wire, err := proto.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			var decoded OpenRequest
			if err := proto.Unmarshal(wire, &decoded); err != nil {
				t.Fatal(err)
			}
			if !proto.Equal(request, &decoded) {
				t.Fatalf("Open request changed after protobuf round trip: %v", &decoded)
			}
		})
	}
}

func TestStartupResponseOptionalPresence(t *testing.T) {
	for _, response := range []proto.Message{
		&OpenResponse{},
		&OpenResponse{WindowId: proto.Uint64(0), Pid: proto.Uint32(0)},
		&LaunchApplicationResponse{},
		&LaunchApplicationResponse{WindowId: proto.Uint64(0), Pid: proto.Uint32(0)},
		&OpenUriResponse{},
		&OpenUriResponse{WindowId: proto.Uint64(0), Pid: proto.Uint32(0)},
	} {
		t.Run(string(response.ProtoReflect().Descriptor().Name()), func(t *testing.T) {
			wire, err := proto.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
			decoded := response.ProtoReflect().New().Interface()
			if err := proto.Unmarshal(wire, decoded); err != nil {
				t.Fatal(err)
			}
			for _, name := range []protoreflect.Name{"window_id", "pid"} {
				field := response.ProtoReflect().Descriptor().Fields().ByName(name)
				if field == nil || !field.HasOptionalKeyword() || !field.HasPresence() {
					t.Fatalf("%s must retain explicit optional presence", name)
				}
				if got, want := decoded.ProtoReflect().Has(field), response.ProtoReflect().Has(field); got != want {
					t.Fatalf("%s presence = %v after protobuf round trip, want %v", name, got, want)
				}
			}
			if !proto.Equal(response, decoded) {
				t.Fatalf("startup response changed after protobuf round trip: %v", decoded)
			}
		})
	}
}
