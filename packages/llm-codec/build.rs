fn main() {
    let root = std::path::PathBuf::from(std::env::var("CARGO_MANIFEST_DIR").unwrap());
    let output = std::path::PathBuf::from(std::env::var("OUT_DIR").unwrap());
    let config = cbindgen::Config::from_file(root.join("cbindgen.toml")).unwrap();
    cbindgen::Builder::new()
        .with_crate(root)
        .with_config(config)
        .generate()
        .expect("generate llm-codec C header")
        .write_to_file(output.join("llm_codec.h"));
    println!("cargo:rerun-if-changed=src");
    println!("cargo:rerun-if-changed=cbindgen.toml");
}
