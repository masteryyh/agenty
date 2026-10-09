use std::collections::BTreeMap;
use std::io::Write;
use std::path::Path;

use chrono::{DateTime, Local};
use minijinja::value::{Kwargs, Value as JinjaValue};
use minijinja::{AutoEscape, Environment, ErrorKind, State};
use serde::{Deserialize, Serialize};
use serde_json::{Map, Value};

use crate::error::{read_limited, read_optional_json, Error, ErrorCode, Result};
use crate::{gguf, LoadOptions};

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TemplateInfo {
    pub name: String,
    pub source: String,
    pub text: String,
}

#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct RenderRequest {
    #[serde(default)]
    pub system_prompt: String,
    pub messages: Vec<RenderMessage>,
    #[serde(default)]
    pub tools: Vec<Value>,
    pub reasoning_effort: Option<String>,
    #[serde(default)]
    pub add_generation_prompt: bool,
    pub template_name: Option<String>,
    #[serde(default)]
    pub template_kwargs: BTreeMap<String, Value>,
    /// RFC3339 clock override for templates using strftime_now.
    pub current_time: Option<String>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct RenderMessage {
    pub role: String,
    #[serde(default)]
    pub content: Value,
    pub reasoning_content: Option<String>,
    #[serde(default)]
    pub tool_calls: Vec<ToolCall>,
    pub tool_call_id: Option<String>,
    pub name: Option<String>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct ToolCall {
    pub id: String,
    pub function: FunctionCall,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct FunctionCall {
    pub name: String,
    pub arguments: Map<String, Value>,
}

pub(crate) struct Templates {
    environment: Environment<'static>,
    infos: BTreeMap<String, TemplateInfo>,
}

pub fn resolve_templates(directory: &Path, options: &LoadOptions) -> Result<Vec<TemplateInfo>> {
    if !directory.is_dir() {
        return Err(Error::new(
            ErrorCode::ModelNotFound,
            "model directory not found",
        ));
    }
    let config = read_optional_json(&directory.join("tokenizer_config.json"))?.unwrap_or_default();
    let mut infos = discover_files(directory, &config)?;
    if infos.is_empty() {
        let path = gguf::select_file(directory, options.gguf_file.as_deref())?;
        if let Some(path) = path {
            add_gguf(&mut infos, &gguf::read(&path)?, &path)?;
        }
    }
    Ok(Templates::compile(infos)?.info())
}

impl Templates {
    pub(crate) fn load(
        directory: &Path,
        config: &Value,
        gguf: &gguf::Metadata,
        gguf_path: Option<&Path>,
    ) -> Result<Self> {
        let mut infos = discover_files(directory, config)?;
        if infos.is_empty() {
            if let Some(path) = gguf_path {
                add_gguf(&mut infos, gguf, path)?;
            }
        }
        Self::compile(infos)
    }

    fn compile(infos: BTreeMap<String, TemplateInfo>) -> Result<Self> {
        if infos.is_empty() {
            return Err(Error::new(
                ErrorCode::TemplateNotFound,
                "no supported chat template found",
            ));
        }
        if infos.len() > 64
            || infos.values().map(|info| info.text.len()).sum::<usize>()
                > 8 * crate::MAX_TEMPLATE_BYTES
        {
            return Err(Error::new(
                ErrorCode::ResourceLimit,
                "chat template collection exceeds size limit",
            ));
        }
        let mut environment = Environment::new();
        environment.set_auto_escape_callback(|_| AutoEscape::None);
        environment.set_trim_blocks(true);
        environment.set_lstrip_blocks(true);
        environment.set_fuel(Some(1_000_000));
        environment.set_recursion_limit(100);
        environment
            .set_unknown_method_callback(minijinja_contrib::pycompat::unknown_method_callback);
        environment.add_filter("tojson", to_json);
        environment.add_function(
            "raise_exception",
            |message: String| -> std::result::Result<String, minijinja::Error> {
                Err(minijinja::Error::new(ErrorKind::InvalidOperation, message))
            },
        );
        environment.add_function(
            "strftime_now",
            |state: &State, format: String| -> std::result::Result<String, minijinja::Error> {
                let now = state
                    .lookup("__llm_codec_current_time")
                    .and_then(|v| v.as_str().map(str::to_owned))
                    .unwrap_or_default();
                let date = DateTime::parse_from_rfc3339(&now).map_err(|e| {
                    minijinja::Error::new(ErrorKind::InvalidOperation, e.to_string())
                })?;
                if chrono::format::StrftimeItems::new(&format)
                    .any(|item| matches!(item, chrono::format::Item::Error))
                {
                    return Err(minijinja::Error::new(
                        ErrorKind::InvalidOperation,
                        "invalid date format",
                    ));
                }
                Ok(date.format(&format).to_string())
            },
        );
        for (name, info) in &infos {
            environment
                .add_template_owned(name.clone(), info.text.clone())
                .map_err(|e| {
                    let code = if e.to_string().contains("unknown statement") {
                        ErrorCode::UnsupportedTemplateFeature
                    } else {
                        ErrorCode::TemplateSyntax
                    };
                    Error::new(code, format!("{} [{}]: {e:#}", info.source, name))
                })?;
        }
        Ok(Self { environment, infos })
    }

    pub(crate) fn info(&self) -> Vec<TemplateInfo> {
        self.infos.values().cloned().collect()
    }

    pub(crate) fn render(
        &self,
        request: &RenderRequest,
        special_tokens: &BTreeMap<String, Value>,
    ) -> Result<String> {
        let name = if let Some(name) = request.template_name.as_deref() {
            name
        } else if !request.tools.is_empty() && self.infos.contains_key("tool_use") {
            "tool_use"
        } else if self.infos.contains_key("default") {
            "default"
        } else if self.infos.len() == 1 {
            self.infos.keys().next().unwrap()
        } else {
            return Err(Error::new(
                ErrorCode::AmbiguousTemplate,
                "specify templateName for this model",
            ));
        };
        let template = self
            .environment
            .get_template(name)
            .map_err(|e| Error::new(ErrorCode::TemplateNotFound, e.to_string()))?;
        let mut context = Map::new();
        context.insert(
            "messages".to_owned(),
            Value::Array(normalize_messages(request)?),
        );
        context.insert(
            "tools".to_owned(),
            if request.tools.is_empty() {
                Value::Null
            } else {
                validate_tools(&request.tools)?;
                Value::Array(request.tools.clone())
            },
        );
        context.insert(
            "add_generation_prompt".to_owned(),
            Value::Bool(request.add_generation_prompt),
        );
        for (name, value) in special_tokens {
            context.insert(name.clone(), value.clone());
        }
        for (name, value) in &request.template_kwargs {
            if context.contains_key(name)
                || name.starts_with("__llm_codec_")
                || name.ends_with("_token")
                || matches!(
                    name.as_str(),
                    "raise_exception" | "strftime_now" | "reasoning_effort"
                )
            {
                return Err(Error::new(
                    ErrorCode::InvalidRequest,
                    format!("templateKwargs cannot override {name}"),
                ));
            }
            context.insert(name.clone(), value.clone());
        }
        if let Some(effort) = request.reasoning_effort.as_deref() {
            if !matches!(effort, "off" | "low" | "medium" | "high" | "xhigh" | "max") {
                return Err(Error::new(
                    ErrorCode::InvalidRequest,
                    "unknown reasoningEffort",
                ));
            }
            let variables = template.undeclared_variables(false);
            if variables.contains("reasoning_effort") {
                context.insert(
                    "reasoning_effort".to_owned(),
                    Value::String(effort.to_owned()),
                );
            } else if effort == "off" && variables.contains("enable_thinking") {
                if context
                    .get("enable_thinking")
                    .is_some_and(|v| v != &Value::Bool(false))
                {
                    return Err(Error::new(
                        ErrorCode::InvalidRequest,
                        "reasoningEffort conflicts with enable_thinking",
                    ));
                }
                context.insert("enable_thinking".to_owned(), Value::Bool(false));
            } else {
                return Err(Error::new(ErrorCode::UnsupportedOption, "template does not expose reasoning_effort; use its explicit templateKwargs (for example enable_thinking)"));
            }
        }
        let now = request
            .current_time
            .clone()
            .unwrap_or_else(|| Local::now().to_rfc3339());
        DateTime::parse_from_rfc3339(&now)
            .map_err(|e| Error::new(ErrorCode::InvalidRequest, format!("currentTime: {e}")))?;
        context.insert("__llm_codec_current_time".to_owned(), Value::String(now));
        let mut output = LimitedOutput {
            bytes: Vec::new(),
            exceeded: false,
        };
        template
            .render_captured_to(&context, &mut output)
            .map_err(|e| {
                let code = if output.exceeded || e.kind() == ErrorKind::OutOfFuel {
                    ErrorCode::ResourceLimit
                } else if matches!(
                    e.kind(),
                    ErrorKind::UnknownMethod
                        | ErrorKind::UnknownFilter
                        | ErrorKind::UnknownFunction
                ) {
                    ErrorCode::UnsupportedTemplateFeature
                } else {
                    ErrorCode::TemplateRender
                };
                Error::new(code, format!("{} [{name}]: {e:#}", self.infos[name].source))
            })?;
        String::from_utf8(output.bytes).map_err(|e| Error::new(ErrorCode::Internal, e.to_string()))
    }
}

fn insert(
    infos: &mut BTreeMap<String, TemplateInfo>,
    name: String,
    source: String,
    text: String,
) -> Result<()> {
    if name.is_empty() || text.is_empty() {
        return Err(Error::new(
            ErrorCode::TemplateSyntax,
            format!("{source}: template name and text must be nonempty"),
        ));
    }
    if text.len() > crate::MAX_TEMPLATE_BYTES {
        return Err(Error::new(
            ErrorCode::ResourceLimit,
            format!("{source}: template exceeds size limit"),
        ));
    }
    infos.insert(name.clone(), TemplateInfo { name, source, text });
    Ok(())
}

fn discover_files(directory: &Path, config: &Value) -> Result<BTreeMap<String, TemplateInfo>> {
    let mut infos = BTreeMap::new();
    let source = directory
        .join("tokenizer_config.json")
        .display()
        .to_string();
    if let Some(value) = config.get("chat_template").filter(|v| !v.is_null()) {
        if let Some(text) = value.as_str() {
            insert(
                &mut infos,
                "default".to_owned(),
                source.clone(),
                text.to_owned(),
            )?;
        } else if let Some(entries) = value.as_array() {
            for entry in entries {
                let name = entry.get("name").and_then(Value::as_str).ok_or_else(|| {
                    Error::new(
                        ErrorCode::InvalidJson,
                        "chat_template entry is missing name",
                    )
                })?;
                let text = entry
                    .get("template")
                    .and_then(Value::as_str)
                    .ok_or_else(|| {
                        Error::new(
                            ErrorCode::InvalidJson,
                            "chat_template entry is missing template",
                        )
                    })?;
                if infos.contains_key(name) {
                    return Err(Error::new(
                        ErrorCode::AmbiguousTemplate,
                        format!("duplicate template name {name}"),
                    ));
                }
                insert(&mut infos, name.to_owned(), source.clone(), text.to_owned())?;
            }
        } else {
            return Err(Error::new(
                ErrorCode::InvalidJson,
                "chat_template must be a string or a named template array",
            ));
        }
    }
    let path = directory.join("chat_template.jinja");
    match read_limited(&path, crate::MAX_TEMPLATE_BYTES) {
        Ok(bytes) => insert(
            &mut infos,
            "default".to_owned(),
            path.display().to_string(),
            String::from_utf8(bytes)
                .map_err(|e| Error::new(ErrorCode::InvalidUtf8, e.to_string()))?,
        )?,
        Err(e) if e.code == ErrorCode::ModelNotFound => {}
        Err(e) => return Err(e),
    }
    let additional = directory.join("additional_chat_templates");
    match std::fs::read_dir(&additional) {
        Ok(entries) => {
            for entry in entries {
                let path = entry
                    .map_err(|e| Error::new(ErrorCode::Io, e.to_string()))?
                    .path();
                if path.extension().is_none_or(|e| e != "jinja") {
                    continue;
                }
                let name = path.file_stem().and_then(|s| s.to_str()).ok_or_else(|| {
                    Error::new(ErrorCode::InvalidUtf8, "invalid template filename")
                })?;
                if name == "default" {
                    return Err(Error::new(
                        ErrorCode::AmbiguousTemplate,
                        "default template belongs in chat_template.jinja",
                    ));
                }
                let text = String::from_utf8(read_limited(&path, crate::MAX_TEMPLATE_BYTES)?)
                    .map_err(|e| Error::new(ErrorCode::InvalidUtf8, e.to_string()))?;
                insert(
                    &mut infos,
                    name.to_owned(),
                    path.display().to_string(),
                    text,
                )?;
                if infos.len() > 64 {
                    return Err(Error::new(ErrorCode::ResourceLimit, "too many templates"));
                }
            }
        }
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
        Err(e) => {
            return Err(Error::new(
                ErrorCode::Io,
                format!("{}: {e}", additional.display()),
            ))
        }
    }
    Ok(infos)
}

fn add_gguf(
    infos: &mut BTreeMap<String, TemplateInfo>,
    metadata: &gguf::Metadata,
    path: &Path,
) -> Result<()> {
    for (key, value) in metadata {
        let name = if key == "tokenizer.chat_template" {
            "default"
        } else if let Some(name) = key.strip_prefix("tokenizer.chat_template.") {
            name
        } else {
            continue;
        };
        insert(
            infos,
            name.to_owned(),
            format!("{}:{key}", path.display()),
            value.as_str().unwrap().to_owned(),
        )?;
    }
    Ok(())
}

fn normalize_messages(request: &RenderRequest) -> Result<Vec<Value>> {
    let mut messages = Vec::new();
    let mut system_parts = Vec::new();
    if !request.system_prompt.is_empty() {
        system_parts.push(request.system_prompt.clone());
    }
    for message in &request.messages {
        if !matches!(
            message.role.as_str(),
            "system" | "user" | "assistant" | "tool"
        ) {
            return Err(Error::new(
                ErrorCode::InvalidRequest,
                format!("unknown message role {}", message.role),
            ));
        }
        let content = if let Some(text) = message.content.as_str() {
            text
        } else if message.content.is_null() && message.role == "assistant" {
            ""
        } else {
            return Err(Error::new(
                ErrorCode::UnsupportedContent,
                "message content must be text; multimodal content is not supported",
            ));
        };
        if message.role != "assistant"
            && (!message.tool_calls.is_empty() || message.reasoning_content.is_some())
        {
            return Err(Error::new(
                ErrorCode::InvalidRequest,
                "toolCalls and reasoningContent require assistant role",
            ));
        }
        if message.role == "tool" && message.tool_call_id.as_deref().is_none_or(str::is_empty) {
            return Err(Error::new(
                ErrorCode::InvalidRequest,
                "tool message requires toolCallId",
            ));
        }
        if message.role != "tool" && message.tool_call_id.is_some() {
            return Err(Error::new(
                ErrorCode::InvalidRequest,
                "toolCallId requires tool role",
            ));
        }
        if message.role == "system" {
            if !content.is_empty() {
                system_parts.push(content.to_owned());
            }
            continue;
        }
        let mut value = Map::new();
        value.insert("role".to_owned(), Value::String(message.role.clone()));
        value.insert("content".to_owned(), Value::String(content.to_owned()));
        if let Some(reasoning) = &message.reasoning_content {
            value.insert(
                "reasoning_content".to_owned(),
                Value::String(reasoning.clone()),
            );
        }
        if let Some(id) = &message.tool_call_id {
            value.insert("tool_call_id".to_owned(), Value::String(id.clone()));
        }
        if let Some(name) = &message.name {
            value.insert("name".to_owned(), Value::String(name.clone()));
        }
        if !message.tool_calls.is_empty() {
            let mut calls = Vec::new();
            for call in &message.tool_calls {
                if call.id.is_empty() || call.function.name.is_empty() {
                    return Err(Error::new(
                        ErrorCode::InvalidRequest,
                        "tool call ID and function name must be nonempty",
                    ));
                }
                calls.push(
                    serde_json::json!({"id":call.id,"type":"function","function":call.function}),
                );
            }
            value.insert("tool_calls".to_owned(), Value::Array(calls));
        }
        messages.push(Value::Object(value));
    }
    if !system_parts.is_empty() {
        messages.insert(
            0,
            serde_json::json!({"role":"system","content":system_parts.join("\n\n")}),
        );
    }
    Ok(messages)
}

fn validate_tools(tools: &[Value]) -> Result<()> {
    for tool in tools {
        if tool.get("type").and_then(Value::as_str) != Some("function")
            || tool
                .get("function")
                .and_then(|v| v.get("name"))
                .and_then(Value::as_str)
                .is_none_or(str::is_empty)
            || !tool
                .get("function")
                .and_then(|v| v.get("parameters"))
                .is_some_and(Value::is_object)
        {
            return Err(Error::new(
                ErrorCode::InvalidRequest,
                "tools must contain function definitions with a name and parameters JSON Schema",
            ));
        }
    }
    Ok(())
}

struct LimitedOutput {
    bytes: Vec<u8>,
    exceeded: bool,
}

impl Write for LimitedOutput {
    fn write(&mut self, bytes: &[u8]) -> std::io::Result<usize> {
        if bytes.len() > crate::MAX_OUTPUT_BYTES - self.bytes.len() {
            self.exceeded = true;
            return Err(std::io::Error::other("rendered output exceeds size limit"));
        }
        self.bytes.extend_from_slice(bytes);
        Ok(bytes.len())
    }
    fn flush(&mut self) -> std::io::Result<()> {
        Ok(())
    }
}

fn to_json(value: JinjaValue, kwargs: Kwargs) -> std::result::Result<String, minijinja::Error> {
    let ensure_ascii = kwargs.get::<Option<bool>>("ensure_ascii")?.unwrap_or(false);
    let sort_keys = kwargs.get::<Option<bool>>("sort_keys")?.unwrap_or(false);
    let indent = kwargs
        .get::<Option<JinjaValue>>("indent")?
        .map(serde_json::to_value)
        .transpose()
        .map_err(|e| minijinja::Error::new(ErrorKind::InvalidOperation, e.to_string()))?;
    let separators = kwargs.get::<Option<Vec<String>>>("separators")?;
    kwargs.assert_all_used()?;
    let mut value = serde_json::to_value(value)
        .map_err(|e| minijinja::Error::new(ErrorKind::InvalidOperation, e.to_string()))?;
    if sort_keys {
        sort_json_keys(&mut value);
    }
    let indent = match indent {
        None | Some(Value::Null) => None,
        Some(Value::String(s)) => Some(s),
        Some(Value::Number(n)) => {
            Some(" ".repeat(n.as_u64().filter(|n| *n <= 32).ok_or_else(|| {
                minijinja::Error::new(
                    ErrorKind::InvalidOperation,
                    "indent must be 0..32 or a string",
                )
            })? as usize))
        }
        _ => {
            return Err(minijinja::Error::new(
                ErrorKind::InvalidOperation,
                "invalid indent",
            ))
        }
    };
    let separators = match separators {
        Some(v) if v.len() == 2 => (v[0].clone(), v[1].clone()),
        Some(_) => {
            return Err(minijinja::Error::new(
                ErrorKind::InvalidOperation,
                "separators must have two strings",
            ))
        }
        None => (
            if indent.is_some() { "," } else { ", " }.to_owned(),
            ": ".to_owned(),
        ),
    };
    if indent.as_ref().is_some_and(|s| s.len() > 128)
        || separators.0.len() > 128
        || separators.1.len() > 128
    {
        return Err(minijinja::Error::new(
            ErrorKind::InvalidOperation,
            "JSON formatting option exceeds size limit",
        ));
    }
    let mut output = String::new();
    write_json(
        &value,
        &mut output,
        indent.as_deref(),
        &separators,
        0,
        ensure_ascii,
    )?;
    Ok(output)
}

fn sort_json_keys(value: &mut Value) {
    match value {
        Value::Object(map) => {
            map.sort_keys();
            for value in map.values_mut() {
                sort_json_keys(value);
            }
        }
        Value::Array(values) => {
            for value in values {
                sort_json_keys(value);
            }
        }
        _ => {}
    }
}

fn write_json(
    value: &Value,
    output: &mut String,
    indent: Option<&str>,
    separators: &(String, String),
    depth: usize,
    ascii: bool,
) -> std::result::Result<(), minijinja::Error> {
    if depth > 100 || output.len() > crate::MAX_OUTPUT_BYTES {
        return Err(minijinja::Error::new(
            ErrorKind::InvalidOperation,
            "JSON output exceeds resource limit",
        ));
    }
    let newline = |output: &mut String, depth| {
        if let Some(indent) = indent {
            output.push('\n');
            for _ in 0..depth {
                output.push_str(indent);
            }
        }
    };
    match value {
        Value::Array(values) => {
            output.push('[');
            for (i, value) in values.iter().enumerate() {
                if i > 0 {
                    output.push_str(&separators.0);
                }
                newline(output, depth + 1);
                write_json(value, output, indent, separators, depth + 1, ascii)?;
            }
            if !values.is_empty() {
                newline(output, depth);
            }
            output.push(']');
        }
        Value::Object(values) => {
            output.push('{');
            for (i, (key, value)) in values.iter().enumerate() {
                if i > 0 {
                    output.push_str(&separators.0);
                }
                newline(output, depth + 1);
                write_json(
                    &Value::String(key.clone()),
                    output,
                    indent,
                    separators,
                    depth + 1,
                    ascii,
                )?;
                output.push_str(&separators.1);
                write_json(value, output, indent, separators, depth + 1, ascii)?;
            }
            if !values.is_empty() {
                newline(output, depth);
            }
            output.push('}');
        }
        Value::Number(number) if number.is_f64() => {
            output.push_str(&python_float(number));
        }
        _ => {
            let text = serde_json::to_string(value)
                .map_err(|e| minijinja::Error::new(ErrorKind::InvalidOperation, e.to_string()))?;
            if ascii {
                for c in text.chars() {
                    if c.is_ascii() {
                        output.push(c);
                    } else {
                        for unit in c.encode_utf16(&mut [0; 2]) {
                            output.push_str(&format!("\\u{unit:04x}"));
                        }
                    }
                }
            } else {
                output.push_str(&text);
            }
        }
    }
    Ok(())
}

// serde_json and Python both produce shortest round-trippable digits, but
// Python switches notation at different exponents and pads the exponent.
fn python_float(number: &serde_json::Number) -> String {
    let text = number.to_string();
    let (sign, unsigned) = text
        .strip_prefix('-')
        .map_or(("", text.as_str()), |s| ("-", s));
    let (mantissa, exponent) = unsigned
        .split_once('e')
        .map_or((unsigned, 0), |(m, e)| (m, e.parse::<i32>().unwrap()));
    let decimal = mantissa.find('.').unwrap_or(mantissa.len()) as i32;
    let digits = mantissa.replace('.', "");
    let first = digits.find(|c| c != '0');
    let Some(first) = first else {
        return format!("{sign}0.0");
    };
    let power = exponent + decimal - first as i32 - 1;
    let digits = digits[first..].trim_end_matches('0');
    if !(-4..16).contains(&power) {
        let fraction = if digits.len() > 1 {
            format!(".{}", &digits[1..])
        } else {
            String::new()
        };
        let exponent = format!("{}{:02}", if power >= 0 { "+" } else { "-" }, power.abs());
        format!("{sign}{}{fraction}e{exponent}", &digits[..1])
    } else {
        let point = power + 1;
        if point <= 0 {
            format!("{sign}0.{}{digits}", "0".repeat((-point) as usize))
        } else if point as usize >= digits.len() {
            format!(
                "{sign}{digits}{}.0",
                "0".repeat(point as usize - digits.len())
            )
        } else {
            format!(
                "{sign}{}.{}",
                &digits[..point as usize],
                &digits[point as usize..]
            )
        }
    }
}
