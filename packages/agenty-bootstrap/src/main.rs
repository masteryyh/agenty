mod progress;
mod supervisor;

fn main() {
    std::process::exit(supervisor::run());
}
