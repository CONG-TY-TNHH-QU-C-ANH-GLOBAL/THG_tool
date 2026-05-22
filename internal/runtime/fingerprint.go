package runtime

import (
	"bytes"
	"math/rand"
	"text/template"
)

// Fingerprint defines the browser identity injected into Chrome before page load.
// All values are deterministic per accountID so the fingerprint is stable across sessions.
type Fingerprint struct {
	UserAgent           string
	Language            string
	Platform            string
	TimezoneID          string
	ScreenWidth         int
	ScreenHeight        int
	WebGLVendor         string
	WebGLRenderer       string
	CanvasNoise         float64
	HardwareConcurrency int
	DeviceMemory        float64
	AudioContextNoise   float64
	PluginCount         int
	MaxTouchPoints      int
	ColorDepth          int
	PixelRatio          float64
}

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 11.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
}

var pixelRatios = []float64{1.0, 1.0, 1.25, 1.5}
var hwConcurrencies = []int{4, 8, 8, 12}
var deviceMemories = []float64{4, 8, 8, 16}

// DefaultFingerprint returns a deterministic fingerprint for the given accountID.
func DefaultFingerprint(accountID int64) Fingerprint {
	rng := rand.New(rand.NewSource(accountID * 0xdeadbeef))
	pick := func(n int) int { return rng.Intn(n) }

	return Fingerprint{
		UserAgent:           userAgents[pick(len(userAgents))],
		Language:            "vi-VN,vi;q=0.9,en-US;q=0.8,en;q=0.7",
		Platform:            "Win32",
		TimezoneID:          "Asia/Ho_Chi_Minh",
		ScreenWidth:         1280,
		ScreenHeight:        800,
		WebGLVendor:         "Intel Inc.",
		WebGLRenderer:       "Intel(R) HD Graphics 620",
		CanvasNoise:         0.0001 + float64(accountID%10)*0.00001,
		HardwareConcurrency: hwConcurrencies[pick(len(hwConcurrencies))],
		DeviceMemory:        deviceMemories[pick(len(deviceMemories))],
		AudioContextNoise:   0.00001 + float64(pick(10))*0.000001,
		PluginCount:         3,
		MaxTouchPoints:      0,
		ColorDepth:          24,
		PixelRatio:          pixelRatios[pick(len(pixelRatios))],
	}
}

var fingerprintTmpl = template.Must(template.New("fp").Parse(`
(function() {
  // ── navigator overrides ─────────────────────────────────────────────────────
  try { delete Object.getPrototypeOf(navigator).webdriver; } catch(e) {}
  Object.defineProperty(navigator, 'userAgent',            { get: () => '{{.UserAgent}}' });
  Object.defineProperty(navigator, 'appVersion',           { get: () => '{{.UserAgent}}'.replace('Mozilla/','') });
  Object.defineProperty(navigator, 'platform',             { get: () => '{{.Platform}}' });
  Object.defineProperty(navigator, 'language',             { get: () => '{{.Language}}'.split(',')[0] });
  Object.defineProperty(navigator, 'languages',            { get: () => '{{.Language}}'.split(',') });
  Object.defineProperty(navigator, 'hardwareConcurrency',  { get: () => {{.HardwareConcurrency}} });
  Object.defineProperty(navigator, 'deviceMemory',         { get: () => {{.DeviceMemory}} });
  Object.defineProperty(navigator, 'maxTouchPoints',       { get: () => {{.MaxTouchPoints}} });

  // ── navigator.plugins ──────────────────────────────────────────────────────
  const fakePlugins = [
    {name:'Chrome PDF Plugin', filename:'internal-pdf-viewer', description:'Portable Document Format', length:1},
    {name:'Chrome PDF Viewer', filename:'mhjfbmdgcfjbbpaeojofohoefgiehjai', description:'', length:1},
    {name:'Native Client',    filename:'internal-nacl-plugin', description:'', length:2},
  ].slice(0, {{.PluginCount}});
  fakePlugins.refresh = function(){};
  fakePlugins.namedItem = function(n){ return this.find(p=>p.name===n)||null; };
  fakePlugins.item      = function(i){ return this[i]||null; };
  Object.defineProperty(navigator, 'plugins', { get: () => fakePlugins });

  // ── screen ─────────────────────────────────────────────────────────────────
  Object.defineProperty(screen, 'width',      { get: () => {{.ScreenWidth}} });
  Object.defineProperty(screen, 'height',     { get: () => {{.ScreenHeight}} });
  Object.defineProperty(screen, 'colorDepth', { get: () => {{.ColorDepth}} });
  Object.defineProperty(screen, 'pixelDepth', { get: () => {{.ColorDepth}} });
  Object.defineProperty(window, 'devicePixelRatio', { get: () => {{.PixelRatio}} });

  // ── timezone ───────────────────────────────────────────────────────────────
  const _origDTF = Intl.DateTimeFormat;
  function PatchedDTF(loc, opts) {
    if (!opts) opts = {};
    if (!opts.timeZone) opts.timeZone = '{{.TimezoneID}}';
    return new _origDTF(loc, opts);
  }
  PatchedDTF.prototype = _origDTF.prototype;
  PatchedDTF.supportedLocalesOf = _origDTF.supportedLocalesOf;
  Intl.DateTimeFormat = PatchedDTF;

  // ── WebGL vendor / renderer ─────────────────────────────────────────────────
  const _getParam = WebGLRenderingContext.prototype.getParameter;
  WebGLRenderingContext.prototype.getParameter = function(p) {
    if (p === 37445) return '{{.WebGLVendor}}';
    if (p === 37446) return '{{.WebGLRenderer}}';
    return _getParam.call(this, p);
  };
  if (window.WebGL2RenderingContext) {
    const _get2 = WebGL2RenderingContext.prototype.getParameter;
    WebGL2RenderingContext.prototype.getParameter = function(p) {
      if (p === 37445) return '{{.WebGLVendor}}';
      if (p === 37446) return '{{.WebGLRenderer}}';
      return _get2.call(this, p);
    };
  }

  // ── Canvas noise ───────────────────────────────────────────────────────────
  const _toDataURL = HTMLCanvasElement.prototype.toDataURL;
  HTMLCanvasElement.prototype.toDataURL = function(type, q) {
    const ctx2d = this.Leads().GetContext('2d');
    if (ctx2d) {
      const imgd = ctx2d.getImageData(0,0,1,1);
      imgd.data[0] = Math.max(0, imgd.data[0] + Math.floor({{.CanvasNoise}} * 255));
      ctx2d.putImageData(imgd,0,0);
    }
    return _toDataURL.call(this, type, q);
  };

  // ── AudioContext noise ──────────────────────────────────────────────────────
  if (window.AudioBuffer) {
    const _gcData = AudioBuffer.prototype.getChannelData;
    AudioBuffer.prototype.getChannelData = function(ch) {
      const data = _gcData.call(this, ch);
      for (let i = 0; i < data.length; i += 100) {
        data[i] += {{.AudioContextNoise}} * (Math.random() - 0.5);
      }
      return data;
    };
  }
})();
`))

// BuildInjectionScript renders the JavaScript fingerprint override script.
func BuildInjectionScript(fp Fingerprint) string {
	var buf bytes.Buffer
	if err := fingerprintTmpl.Execute(&buf, fp); err != nil {
		return ""
	}
	return buf.String()
}
