package browsergateway

// Provider identifies the browser execution backend behind a THG account.
// Agent Brain and server handlers should depend on provider capabilities,
// not on one concrete transport implementation.
type Provider string

const (
	ProviderChromeExtension Provider = "chrome_extension_facebook"
)

const (
	KindExtensionConnector      = "extension_connector"
	TransportChromeExtension    = "chrome_extension"
	DefaultChromeConnectorName  = "THG Chrome"
	DefaultExtensionDisplayName = "THG Chrome Extension"
)

// Stream statuses reported by browser providers.
const (
	StreamConnectorOnline       = "connector_online"
	StreamChromeNotConnected    = "chrome_not_connected"
	StreamFacebookLoggedIn      = "facebook_logged_in"
	StreamFacebookLoginRequired = "facebook_login_required"
	StreamFacebookHumanRequired = "facebook_human_required"
)

// Capability names used in connector capabilities_json.
const (
	CapabilityStreamFrames      = "dashboard_stream"
	CapabilityFacebookIdentity  = "dom_metadata"
	CapabilityInputRelay        = "input_relay"
	CapabilityCommandPolling    = "command_polling"
	CapabilityOutboxPolling     = "outbox_polling"
	CapabilityOutboundExecutor  = "outbound_executor"
	CapabilityCrawlVisiblePosts = "crawl_visible_posts"
)
