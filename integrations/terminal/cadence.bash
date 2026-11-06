# Cadence Shell Integration for Bash
# Add to ~/.bashrc:
#   eval "$(cadence terminal hook bash)"

if [ -n "$BASH_VERSION" ]; then
  _cadence_bash_prompt() {
    local exit_code=$?
    (cadence context set --editor terminal --shell bash --pid "$$" --pwd "$PWD" --exit-code "$exit_code" >/dev/null 2>&1 &)
  }

  if [[ ";$PROMPT_COMMAND;" != *";_cadence_bash_prompt;"* ]]; then
    PROMPT_COMMAND="_cadence_bash_prompt;${PROMPT_COMMAND:-}"
  fi
fi
