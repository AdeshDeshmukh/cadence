class Cadence < Formula
  desc "Local AI-first voice dictation CLI and developer engine"
  homepage "https://github.com/AdeshDeshmukh/cadence"
  url "https://github.com/AdeshDeshmukh/cadence/archive/refs/tags/v1.0.0.tar.gz"
  sha256 "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
  license "Apache-2.0"
  head "https://github.com/AdeshDeshmukh/cadence.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "./cmd/cadence"
    system "go", "build", *std_go_args(output: bin/"cadenced", ldflags: "-s -w"), "./cmd/cadenced"
  end

  service do
    run [opt_bin/"cadenced"]
    keep_alive true
    log_path var/"log/cadence/cadenced.log"
    error_log_path var/"log/cadence/cadenced.log"
    process_type :interactive
  end

  test do
    assert_match "cadence version", shell_output("#{bin}/cadence version")
    assert_match "cadence - Local AI-first voice dictation CLI", shell_output("#{bin}/cadence --help")
  end
end
