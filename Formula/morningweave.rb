class Morningweave < Formula
  desc "Single-user CLI that builds a scheduled content digest"
  homepage "https://github.com/mbtz/morningweave"
  url "https://github.com/mbtz/morningweave/archive/refs/tags/v1.0.1.tar.gz"
  sha256 "c9f616b00f78196f9c9d1f828bcb20707d64a91b5e8fe1b4b8d1975a5519cdbc"
  version "1.0.1"
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
