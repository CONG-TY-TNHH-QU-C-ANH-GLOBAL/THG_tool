// AutomationChannelRegistry — maps a channel key (facebook|taobao|1688|…) to its adapter
// implementing the AutomationChannelAdapter contract. Platform CORE: the orchestrator resolves
// an adapter by ctx.channel and calls contract methods without any channel knowledge. Pure
// in-memory map; registration validates the adapter against THGAutomation.AdapterContract.
var THGAutomation = globalThis.THGAutomation || (globalThis.THGAutomation = {});
THGAutomation.ChannelRegistry = THGAutomation.ChannelRegistry || (() => {
  const channels = new Map();
  function register(channel, adapter) {
    if (!channel || typeof channel !== 'string') throw new Error('channel_registry: channel must be a non-empty string');
    const contract = THGAutomation.AdapterContract;
    if (contract && contract.assertValid) contract.assertValid(adapter, channel);
    channels.set(channel, adapter);
    return adapter;
  }
  function get(channel) { return channels.get(channel) || null; }
  function has(channel) { return channels.has(channel); }
  function list() { return Array.from(channels.keys()).sort(); }
  function clear() { channels.clear(); } // test isolation only
  return { register, get, has, list, clear };
})();
if (typeof module !== 'undefined' && module.exports) module.exports = THGAutomation.ChannelRegistry;
