//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

func parseServices(value *C.wasmtime_component_val_t) []Service {
	if C.rc_val_kind(value) != 13 {
		return nil
	}
	services := make([]Service, 0, int(C.rc_list_len(value)))
	for i := 0; i < int(C.rc_list_len(value)); i++ {
		record := C.rc_list_get(value, C.size_t(i))
		service := Service{}
		for j := 0; j < int(C.rc_record_len(record)); j++ {
			key := goString(C.rc_record_key(record, C.size_t(j)), C.rc_record_key_len(record, C.size_t(j)))
			field := C.rc_record_val(record, C.size_t(j))
			switch key {
			case "name":
				service.Name = valueString(field)
			case "version":
				service.Version = valueString(field)
			case "priority":
				service.Priority = int32(C.rc_u32(field))
			case "keys":
				service.Keys = parseStrings(field)
			}
		}
		services = append(services, service)
	}
	return services
}

func parseRequirements(value *C.wasmtime_component_val_t) []Requirement {
	if C.rc_val_kind(value) != 13 {
		return nil
	}
	requirements := make([]Requirement, 0, int(C.rc_list_len(value)))
	for i := 0; i < int(C.rc_list_len(value)); i++ {
		record := C.rc_list_get(value, C.size_t(i))
		requirement := Requirement{}
		for j := 0; j < int(C.rc_record_len(record)); j++ {
			key := goString(C.rc_record_key(record, C.size_t(j)), C.rc_record_key_len(record, C.size_t(j)))
			field := C.rc_record_val(record, C.size_t(j))
			switch key {
			case "name":
				requirement.Name = valueString(field)
			case "version":
				requirement.Version = valueString(field)
			case "selection":
				requirement.Selection = uint8(C.rc_u32(field))
			}
		}
		requirements = append(requirements, requirement)
	}
	return requirements
}

func parseCommands(value *C.wasmtime_component_val_t) []Command {
	if C.rc_val_kind(value) != 13 {
		return nil
	}
	commands := make([]Command, 0, int(C.rc_list_len(value)))
	for i := 0; i < int(C.rc_list_len(value)); i++ {
		record := C.rc_list_get(value, C.size_t(i))
		command := Command{}
		for j := 0; j < int(C.rc_record_len(record)); j++ {
			key := goString(C.rc_record_key(record, C.size_t(j)), C.rc_record_key_len(record, C.size_t(j)))
			field := C.rc_record_val(record, C.size_t(j))
			switch key {
			case "name":
				command.Name = valueString(field)
			case "summary":
				command.Summary = valueString(field)
			case "usage":
				command.Usage = valueString(field)
			}
		}
		commands = append(commands, command)
	}
	return commands
}

func parseStrings(value *C.wasmtime_component_val_t) []string {
	if C.rc_val_kind(value) != 13 {
		return nil
	}
	values := make([]string, 0, int(C.rc_list_len(value)))
	for i := 0; i < int(C.rc_list_len(value)); i++ {
		values = append(values, valueString(C.rc_list_get(value, C.size_t(i))))
	}
	return values
}
