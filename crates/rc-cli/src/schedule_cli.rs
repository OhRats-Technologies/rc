use clap::Subcommand;

#[derive(Subcommand)]
pub(crate) enum ScheduleCommand {
    /// List Node-local schedules.
    List {
        device: String,
        #[arg(long)]
        url: Option<String>,
        #[arg(long)]
        token: Option<String>,
    },
    /// Add an exact-argv or portable-shell schedule.
    Add {
        device: String,
        #[arg(long)]
        cron: String,
        #[arg(long)]
        timezone: String,
        #[arg(long = "shell", value_name = "SCRIPT")]
        shell_source: Option<String>,
        #[arg(long, default_value_t = 3600)]
        max_runtime_seconds: u64,
        #[arg(long)]
        url: Option<String>,
        #[arg(long)]
        token: Option<String>,
        #[arg(last = true)]
        command: Vec<String>,
    },
    /// Remove a schedule and revoke its unattended authority.
    Remove {
        device: String,
        id: String,
        #[arg(long)]
        url: Option<String>,
        #[arg(long)]
        token: Option<String>,
    },
    /// Enable a schedule whose authority remains active.
    Enable { device: String, id: String },
    /// Disable a schedule without deleting it.
    Disable { device: String, id: String },
}
