import { registerService } from './registry';
import { facebookServiceModule } from '../../modules/autoflow/service';

let bootstrapped = false;

export function bootstrapServices(): void {
  if (bootstrapped) return;
  bootstrapped = true;
  registerService(facebookServiceModule);
  // Future: registerService(taobaoServiceModule), registerService(_1688ServiceModule)
}
