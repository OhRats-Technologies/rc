use super::*;

#[test]
fn status_requires_explicit_device_and_process_identity() {
    let schema = status_descriptor()["inputSchema"].clone();
    assert_eq!(
        schema["required"],
        serde_json::json!(["deviceId", "processId"])
    );
    assert_eq!(schema["properties"]["deviceId"]["type"], "string");
}

#[test]
fn structured_tools_publish_exact_unbounded_string_schemas() {
    let descriptors = [
        machines_descriptor(),
        image_descriptor(),
        run_descriptor(),
        status_descriptor(),
        input_descriptor(),
        cancel_descriptor(),
    ];
    for descriptor in descriptors {
        assert!(descriptor.get("outputSchema").is_some());
        assert!(!contains_key(&descriptor, "minLength"));
        assert!(!contains_key(&descriptor, "maxLength"));
    }
}

fn contains_key(value: &serde_json::Value, key: &str) -> bool {
    match value {
        serde_json::Value::Object(object) => {
            object.contains_key(key) || object.values().any(|value| contains_key(value, key))
        }
        serde_json::Value::Array(values) => values.iter().any(|value| contains_key(value, key)),
        _ => false,
    }
}

#[test]
fn closed_objects_declare_every_required_field() {
    for descriptor in [
        machines_descriptor(),
        image_descriptor(),
        run_descriptor(),
        status_descriptor(),
        input_descriptor(),
        cancel_descriptor(),
    ] {
        check_closed_objects(&descriptor, descriptor["name"].as_str().unwrap());
    }
}

fn check_closed_objects(value: &serde_json::Value, path: &str) {
    match value {
        serde_json::Value::Object(object) => {
            if object.get("additionalProperties") == Some(&serde_json::json!(false))
                && let Some(required) = object.get("required").and_then(|v| v.as_array())
            {
                for field in required {
                    let field = field.as_str().unwrap();
                    assert!(
                        object
                            .get("properties")
                            .and_then(|v| v.get(field))
                            .is_some(),
                        "{path}: required field {field} is forbidden by additionalProperties=false"
                    );
                }
            }
            for (key, child) in object {
                check_closed_objects(child, &format!("{path}/{key}"));
            }
        }
        serde_json::Value::Array(array) => {
            for (index, child) in array.iter().enumerate() {
                check_closed_objects(child, &format!("{path}/{index}"));
            }
        }
        _ => {}
    }
}

#[test]
fn run_advertises_explicit_execution_modes_and_lifetime() {
    let schema = run_descriptor()["inputSchema"].clone();
    assert_eq!(schema["required"], serde_json::json!(["deviceId"]));
    assert_eq!(schema["properties"]["argv"]["type"], "array");
    assert_eq!(
        schema["properties"]["shell"]["enum"],
        serde_json::json!(["rc", "system"])
    );
    assert_eq!(schema["properties"]["maxRuntimeSeconds"]["minimum"], 1);
}

#[test]
fn stale_status_request_is_missing_device_id() {
    // Exact arguments captured in RC_DEBUG_EVIDENCE.json; no remote call.
    let request = serde_json::json!({
        "processId": "56dd8b94-e58c-48e8-ae77-2f676d45ccaa",
        "cursor": 286,
        "waitSeconds": 1,
    });
    let schema = status_descriptor()["inputSchema"].clone();
    let missing: Vec<_> = schema["required"]
        .as_array()
        .unwrap()
        .iter()
        .filter_map(|field| field.as_str())
        .filter(|field| request.get(*field).is_none())
        .collect();
    assert_eq!(missing, ["deviceId"]);
    // Retain the explicit device boundary instead of adapting to stale discovery.
    assert_eq!(schema["properties"]["deviceId"]["type"], "string");
    assert_eq!(schema["additionalProperties"], false);
}
