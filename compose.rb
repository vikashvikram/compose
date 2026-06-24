class Compose < Formula
  desc "Lightweight native Docker Compose clone for macOS"
  homepage "https://github.com/vikashvikram/compose"
  url "https://github.com/vikashvikram/compose/archive/refs/tags/v1.0.0.tar.gz"
  sha256 "1b939c5b94d609f3cc69c86601ba09c3c48de771cd747c67e10533c2dc11942a"
  license "Apache-2.0"

  depends_on "go" => :build
  depends_on "container"

  def install
    # Compiles from source on install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "main.go"
  end

  test do
    assert_match "macOS Container Compose Tool", shell_output("#{bin}/compose --help")
  end

  def caveats
    <<~EOS
      This tool wraps macOS's native `container` tool, which has been installed as a dependency.
      To initialize and start the background container daemon, run:
        brew services start container
      Or start it on-demand with:
        container system start
    EOS
  end
end
