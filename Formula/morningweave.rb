class Morningweave < Formula
  desc "Single-user CLI that builds a scheduled content digest"
  homepage "https://github.com/mbtz/morningweave"
  url "https://github.com/mbtz/morningweave/archive/refs/tags/v1.0.0.tar.gz"
  sha256 "REPLACE_WITH_SHA256"
  version "1.0.0"
  head "https://github.com/mbtz/morningweave.git", branch: "main"

  depends_on "go" => :build

  def install
    ENV["CGO_ENABLED"] = "0"
    ENV["GOFLAGS"] = "-buildvcs=false"

    ldflags = "-s -w -X morningweave/internal/cli.Version=#{version}"
    system "go", "build", "-trimpath", "-ldflags", ldflags,
           "-o", bin/"morningweave", "./cmd/morningweave"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/morningweave --version").strip
  end
end
