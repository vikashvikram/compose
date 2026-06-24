class Compose < Formula
  desc "Lightweight native Docker Compose clone for macOS"
  homepage "https://github.com/vikashvikram/compose"
  url "https://github.com/vikashvikram/compose/archive/refs/tags/v1.0.0.tar.gz"
  sha256 "REPLACE_WITH_TARBALL_SHA256"
  license "Apache-2.0"

  depends_on "go" => :build

  def install
    # Compiles from source on install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "main.go"
  end

  test do
    assert_match "macOS Container Compose Tool", shell_output("#{bin}/compose --help")
  end
end
