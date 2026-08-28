use wasmtime::component::Val;

pub fn result_value(mut values: Vec<Val>, label: &str) -> Result<Val, String> {
    if values.len() != 1 {
        return Err(format!("{label} returned {} values", values.len()));
    }
    let Val::Result(result) = values.remove(0) else {
        return Err(format!("{label} returned a non-result value"));
    };
    match result {
        Ok(Some(value)) => Ok(*value),
        Ok(None) => Err(format!("{label} returned an empty success value")),
        Err(Some(error)) => match *error {
            Val::String(error) => Err(error),
            _ => Err(format!("{label} returned an invalid error")),
        },
        Err(None) => Err(format!("{label} returned an empty error")),
    }
}

pub fn unit_result(mut values: Vec<Val>, label: &str) -> Result<(), String> {
    if values.len() != 1 {
        return Err(format!("{label} returned {} values", values.len()));
    }
    let Val::Result(result) = values.remove(0) else {
        return Err(format!("{label} returned a non-result value"));
    };
    match result {
        Ok(None) => Ok(()),
        Ok(Some(_)) => Err(format!("{label} returned a non-unit success value")),
        Err(Some(error)) => match *error {
            Val::String(error) => Err(error),
            _ => Err(format!("{label} returned an invalid error")),
        },
        Err(None) => Err(format!("{label} returned an empty error")),
    }
}

pub fn record(value: Val, label: &str) -> Result<Vec<(String, Val)>, String> {
    match value {
        Val::Record(fields) => Ok(fields),
        _ => Err(format!("{label} is not a record")),
    }
}

pub fn field<'a>(fields: &'a [(String, Val)], name: &str) -> Result<&'a Val, String> {
    fields
        .iter()
        .find(|(candidate, _)| candidate == name)
        .map(|(_, value)| value)
        .ok_or_else(|| format!("missing field {name:?}"))
}

pub fn string_field(fields: &[(String, Val)], name: &str) -> Result<String, String> {
    match field(fields, name)? {
        Val::String(value) => Ok(value.clone()),
        _ => Err(format!("field {name:?} is not a string")),
    }
}

pub fn u16_field(fields: &[(String, Val)], name: &str) -> Result<u16, String> {
    match field(fields, name)? {
        Val::U16(value) => Ok(*value),
        _ => Err(format!("field {name:?} is not u16")),
    }
}

pub fn u32_field(fields: &[(String, Val)], name: &str) -> Result<u32, String> {
    match field(fields, name)? {
        Val::U32(value) => Ok(*value),
        _ => Err(format!("field {name:?} is not u32")),
    }
}

pub fn list_field(fields: &[(String, Val)], name: &str) -> Result<Vec<Val>, String> {
    match field(fields, name)? {
        Val::List(value) => Ok(value.clone()),
        _ => Err(format!("field {name:?} is not a list")),
    }
}

pub fn option_record_field(
    fields: &[(String, Val)],
    name: &str,
) -> Result<Option<Vec<(String, Val)>>, String> {
    match field(fields, name)? {
        Val::Option(None) => Ok(None),
        Val::Option(Some(value)) => record((**value).clone(), name).map(Some),
        _ => Err(format!("field {name:?} is not an option")),
    }
}
