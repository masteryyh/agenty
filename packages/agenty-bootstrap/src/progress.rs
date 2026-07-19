use std::io::{self, IsTerminal, Write};
use std::sync::{Arc, Condvar, Mutex};
use std::thread::{self, JoinHandle};
use std::time::Duration;

const FRAME_INTERVAL: Duration = Duration::from_millis(80);
const SPINNER_FRAMES: [&str; 10] = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"];

#[derive(Clone)]
enum Entry {
    Parent(String),
    Child(String),
}

#[derive(Default)]
struct State {
    entries: Vec<Entry>,
    frame: usize,
    dirty: bool,
    done: bool,
}

pub struct ProgressLog {
    interactive: bool,
    shared: Arc<(Mutex<State>, Condvar)>,
    worker: Option<JoinHandle<()>>,
}

impl ProgressLog {
    pub fn new() -> Self {
        Self::with_interactive(io::stderr().is_terminal())
    }

    fn with_interactive(interactive: bool) -> Self {
        let shared = Arc::new((Mutex::new(State::default()), Condvar::new()));
        let worker = interactive.then(|| {
            let shared = Arc::clone(&shared);
            thread::spawn(move || render_loop(shared))
        });
        Self {
            interactive,
            shared,
            worker,
        }
    }

    pub fn parent(&self, message: impl Into<String>) {
        self.push(Entry::Parent(message.into()));
    }

    pub fn child(&self, message: impl Into<String>) {
        self.push(Entry::Child(message.into()));
    }

    fn push(&self, entry: Entry) {
        if !self.interactive {
            match entry {
                Entry::Parent(message) | Entry::Child(message) => eprintln!("  {message}"),
            }
            return;
        }

        let (state, changed) = &*self.shared;
        let mut state = state
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        state.entries.push(entry);
        state.dirty = true;
        changed.notify_one();
    }

    pub fn finish(mut self) {
        self.stop_worker();
    }

    fn stop_worker(&mut self) {
        let Some(worker) = self.worker.take() else {
            return;
        };
        let (state, changed) = &*self.shared;
        {
            let mut state = state
                .lock()
                .unwrap_or_else(|poisoned| poisoned.into_inner());
            state.done = true;
            state.dirty = true;
            changed.notify_one();
        }
        let _ = worker.join();
    }
}

impl Drop for ProgressLog {
    fn drop(&mut self) {
        self.stop_worker();
    }
}

fn render_loop(shared: Arc<(Mutex<State>, Condvar)>) {
    let mut rendered_lines = 0;
    loop {
        let (state, changed) = &*shared;
        let mut state = state
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        if !state.dirty && !state.done {
            let (next, _) = changed
                .wait_timeout(state, FRAME_INTERVAL)
                .unwrap_or_else(|poisoned| poisoned.into_inner());
            state = next;
            state.frame = state.frame.wrapping_add(1);
        }
        let entries = state.entries.clone();
        let frame = state.frame;
        let done = state.done;
        state.dirty = false;
        drop(state);

        let lines = render_lines(&entries, frame, !done);
        redraw(&lines, rendered_lines);
        rendered_lines = lines.len();
        if done {
            break;
        }
    }
}

fn render_lines(entries: &[Entry], frame: usize, spinning: bool) -> Vec<String> {
    let active_parent = spinning
        .then(|| {
            entries
                .iter()
                .rposition(|entry| matches!(entry, Entry::Parent(_)))
        })
        .flatten();

    entries
        .iter()
        .enumerate()
        .map(|(index, entry)| match entry {
            Entry::Parent(message) if active_parent == Some(index) => {
                format!("{} {message}", SPINNER_FRAMES[frame % SPINNER_FRAMES.len()])
            }
            Entry::Parent(message) => format!("  {message}"),
            Entry::Child(message) => format!("    \x1b[2m{message}\x1b[0m"),
        })
        .collect()
}

fn redraw(lines: &[String], previous_line_count: usize) {
    let stderr = io::stderr();
    let mut stderr = stderr.lock();
    if previous_line_count > 0 {
        let _ = write!(stderr, "\x1b[{previous_line_count}A");
    }
    for line in lines {
        let _ = write!(stderr, "\r\x1b[2K{line}\n");
    }
    let _ = stderr.flush();
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn spinner_stays_on_latest_parent_when_children_are_added() {
        let entries = vec![
            Entry::Parent("starting agenty 1.2.3...".to_string()),
            Entry::Parent("checking local binary integrity...".to_string()),
            Entry::Child("checking cli binary integrity".to_string()),
            Entry::Child("cli integrity check passed, skipping extraction.".to_string()),
        ];

        let lines = render_lines(&entries, 0, true);
        assert_eq!(lines[0], "  starting agenty 1.2.3...");
        assert_eq!(lines[1], "⠋ checking local binary integrity...");
        assert_eq!(lines[2], "  \x1b[2mchecking cli binary integrity\x1b[0m");
        assert_eq!(
            lines[3],
            "  \x1b[2mcli integrity check passed, skipping extraction.\x1b[0m"
        );
    }

    #[test]
    fn finishing_removes_the_spinner() {
        let entries = vec![Entry::Parent(
            "local binary not found, extracting...".to_string(),
        )];
        assert_eq!(
            render_lines(&entries, 4, false),
            vec!["  local binary not found, extracting..."]
        );
    }
}
