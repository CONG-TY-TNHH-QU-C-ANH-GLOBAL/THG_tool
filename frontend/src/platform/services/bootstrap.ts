import { registerService } from './registry';
import { facebookServiceModule } from '../../modules/autoflow/service';
import { alibaba1688ServiceModule, taobaoServiceModule } from './stubServices';

let bootstrapped = false;

// Taobao + 1688 are registered as Coming Soon stubs (resolveStatus →
// 'unavailable'). They make the /services page reflect the multi-service
// vision on first paint so users immediately read "THG = platform with
// many services" instead of "THG = Facebook tool".
//
// The backend mirrors this in internal/platform/services/bootstrap.go.
// Replace the stubs with real ServiceModules when those automations land.
export function bootstrapServices(): void {
  if (bootstrapped) return;
  bootstrapped = true;
  registerService(facebookServiceModule);
  registerService(taobaoServiceModule);
  registerService(alibaba1688ServiceModule);
}
