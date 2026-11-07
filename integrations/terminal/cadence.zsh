# Cadence Shell Integration for Zsh
# Add to ~/.zshrc:
#   eval "$(cadence terminal hook zsh)"

if [[ -n "$ZSH_VERSION" ]]; then
  _cadence_zsh_preexec() {
    local cmd="$1"
    export _CADENCE_LAST_CMD="$cmd"
    (cadence context set --editor terminal --shell zsh --pid "$$" --pwd "$PWD" --cmd "$cmd" >/dev/null 2>&1 &)
  }

  _cadence_zsh_precmd() {
    local exit_code=$?
    if [[ -n "$_CADENCE_LAST_CMD" ]]; then
      (cadence context set --editor terminal --shell zsh --pid "$$" --pwd "$PWD" --cmd "$_CADENCE_LAST_CMD" --exit-code "$exit_code" >/dev/null 2>&1 &)
      unset _CADENCE_LAST_CMD
    else
      (cadence context set --editor terminal --shell zsh --pid "$$" --pwd "$PWD" --exit-code "$exit_code" >/dev/null 2>&1 &)
    fi
  }

  autoload -Uz add-zsh-hook
  add-zsh-hook preexec _cadence_zsh_preexec
  add-zsh-hook precmd _cadence_zsh_precmd
fi

