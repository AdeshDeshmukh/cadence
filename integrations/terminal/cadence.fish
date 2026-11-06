# Cadence Shell Integration for Fish
# Add to ~/.config/fish/config.fish:
#   cadence terminal hook fish | source

function _cadence_fish_preexec --on-event fish_preexec
    set -g _CADENCE_LAST_CMD $argv[1]
    command cadence context set --editor terminal --shell fish --pid $fish_pid --pwd "$PWD" --cmd "$argv[1]" >/dev/null 2>&1 &
end

function _cadence_fish_postexec --on-event fish_postexec
    set -l exit_code $status
    if set -q _CADENCE_LAST_CMD
        command cadence context set --editor terminal --shell fish --pid $fish_pid --pwd "$PWD" --cmd "$_CADENCE_LAST_CMD" --exit-code $exit_code >/dev/null 2>&1 &
        set -e _CADENCE_LAST_CMD
    else
        command cadence context set --editor terminal --shell fish --pid $fish_pid --pwd "$PWD" --exit-code $exit_code >/dev/null 2>&1 &
    end
end
