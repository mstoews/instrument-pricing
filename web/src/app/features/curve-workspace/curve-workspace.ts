import {
  ChangeDetectionStrategy,
  Component,
  inject,
  input,
  OnChanges,
  output,
  signal,
  SimpleChanges,
} from '@angular/core';
import { DatePipe } from '@angular/common';
import { form, FormField, pattern, readonly, required, submit } from '@angular/forms/signals';
import { CurveWorkspaceStore } from '../../state/curve-workspace.store';
import { CurvePoint, CurveRateSelection } from '../../core/models';
import { isPricingRate } from '../../core/curves';

@Component({
  selector: 'app-curve-workspace',
  imports: [DatePipe, FormField],
  providers: [CurveWorkspaceStore],
  templateUrl: './curve-workspace.html',
  styleUrl: './curve-workspace.css',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class CurveWorkspace implements OnChanges {
  readonly currency = input('');
  readonly rateSelected = output<CurveRateSelection>();
  readonly store = inject(CurveWorkspaceStore);
  readonly query = signal({ currency: 'USD' });
  readonly currencyForm = form(this.query, (path) => {
    required(path.currency, { message: 'Enter a three-letter currency code, such as USD.' });
    pattern(path.currency, /^[A-Za-z]{3}$/, {
      message: 'Enter a three-letter currency code, such as USD.',
    });
    readonly(path.currency, () => !!this.currency());
  });
  readonly isPricingRate = isPricingRate;
  ngOnChanges(changes: SimpleChanges) {
    if (changes['currency']) {
      this.store.clear();
      this.query.set({ currency: this.currency() || 'USD' });
      this.currencyForm().reset();
    }
  }
  load() {
    return submit(this.currencyForm, async () => {
      this.store.loadCurrency(this.currency() || this.query().currency);
    });
  }
  apply(point: CurvePoint) {
    const curve = this.store.selectedCurve();
    if (!curve || !this.store.points().includes(point) || !isPricingRate(point.zero_rate)) return;
    if (this.currency() && curve.currency !== this.currency()) return;
    this.rateSelected.emit({ curve, point });
  }
}
