import {
  ChangeDetectionStrategy,
  Component,
  computed,
  ElementRef,
  inject,
  input,
  OnChanges,
  output,
  signal,
  SimpleChanges,
  viewChild,
} from '@angular/core';
import { form, FormField, submit, validate } from '@angular/forms/signals';
import { DatePipe } from '@angular/common';
import { actual365Fixed, usableSpot } from './scenario-conventions';
import {
  EuropeanPriceRequest,
  Instrument,
  OptionContract,
  Quote,
  CurveRateSelection,
} from '../../core/models';
import { CurveWorkspace } from '../curve-workspace/curve-workspace';
import { isPricingRate } from '../../core/curves';
import { PricingStore } from '../../state/pricing.store';

const sampleScenario = (): EuropeanPriceRequest => ({
  exercise_style: 'european',
  right_type: 'call',
  strike: '100',
  time_to_expiry: '1',
  spot: '100',
  volatility: '0.20',
  risk_free_rate: '0.05',
  dividend_yield: '0',
});
const decimal = /^-?[0-9]{1,12}(\.[0-9]{1,8})?$/;
const zero = /^-?0+(\.0+)?$/;
const numericFields = [
  {
    key: 'spot',
    label: 'Spot price',
    help: 'Same currency units as strike.',
    constraint: 'positive',
  },
  {
    key: 'strike',
    label: 'Strike price',
    help: 'Same currency units as spot.',
    constraint: 'positive',
  },
  {
    key: 'time_to_expiry',
    label: 'Time to expiry (years)',
    help: '1 = one year; 0 = expiry.',
    constraint: 'nonnegative',
  },
  {
    key: 'volatility',
    label: 'Annualized volatility',
    help: '0.20 = 20%; zero is allowed.',
    constraint: 'nonnegative',
  },
  {
    key: 'risk_free_rate',
    label: 'Risk-free rate',
    help: '0.05 = 5%, continuously compounded.',
    constraint: 'signed',
  },
  {
    key: 'dividend_yield',
    label: 'Dividend yield',
    help: 'Continuously compounded; may be negative.',
    constraint: 'signed',
  },
] as const;

@Component({
  selector: 'app-european-pricer',
  imports: [FormField, DatePipe, CurveWorkspace],
  providers: [PricingStore],
  templateUrl: './european-pricer.html',
  styleUrl: './european-pricer.css',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class EuropeanPricer implements OnChanges {
  private readonly heading = viewChild.required<ElementRef<HTMLHeadingElement>>('pricingHeading');
  focus() {
    this.heading().nativeElement.focus();
  }
  readonly contract = input<OptionContract | null>(null);
  readonly instrument = input<Instrument | null>(null);
  readonly quote = input<Quote | null>(null);
  readonly useSample = output<void>();
  readonly valuation = signal({ date: '' });
  readonly valuationForm = form(this.valuation);
  readonly yearFraction = computed(() =>
    actual365Fixed(this.valuation().date, this.contract()?.expiration ?? ''),
  );
  readonly usableSpot = usableSpot;
  readonly copiedSpot = signal('');
  readonly copiedRate = signal('');
  readonly appliedDate = signal('');

  readonly pricing = inject(PricingStore);
  // Local state is an editable draft only; the store owns all server state.
  readonly model = signal(sampleScenario());
  readonly fields = numericFields;
  readonly scenarioForm = form(this.model, (path) => {
    validate(path.right_type, ({ value }) =>
      value() === 'call' || value() === 'put'
        ? undefined
        : { kind: 'right', message: 'Choose call or put.' },
    );
    for (const field of numericFields) {
      validate(path[field.key], ({ value }) => {
        const text = value();
        let message: string | undefined;
        if (!decimal.test(text))
          message = 'Enter a decimal with up to 12 whole and 8 fractional digits.';
        else if (field.constraint === 'positive' && (text.startsWith('-') || zero.test(text)))
          message = 'Enter a value greater than zero.';
        else if (field.constraint === 'nonnegative' && text.startsWith('-') && !zero.test(text))
          message = 'Enter zero or a positive value.';
        return message ? { kind: 'decimal', message } : undefined;
      });
    }
  });

  ngOnChanges(changes: SimpleChanges) {
    if (changes['contract']) this.reset();
  }
  useQuote(side: 'bid' | 'ask') {
    const quote = this.quote();
    if (
      !this.contract() ||
      !quote ||
      quote.instrument_id !== this.instrument()?.id ||
      !usableSpot(quote[side])
    )
      return;
    this.pricing.clear();
    this.model.update((model) => ({ ...model, spot: quote[side] }));
    this.copiedSpot.set(`Copied ${side} from ${quote.source_id}, observed ${quote.observed_at}.`);
  }
  useCurveRate(selection: CurveRateSelection) {
    if (!isPricingRate(selection.point.zero_rate)) return;
    if (this.contract() && selection.curve.currency !== this.instrument()?.currency) return;
    this.pricing.clear();
    this.model.update((model) => ({ ...model, risk_free_rate: selection.point.zero_rate }));
    this.copiedRate.set(
      `Copied ${selection.curve.currency} ${selection.point.tenor_days}-day rate from ${selection.curve.name}; source ${selection.point.source_id}, observed ${selection.point.observed_at}. This is a fixed scenario assumption; refresh does not update it.`,
    );
    this.focus();
  }
  useDate() {
    const time = this.yearFraction();
    if (time === null) return;
    this.pricing.clear();
    this.model.update((model) => ({ ...model, time_to_expiry: time }));
    this.appliedDate.set(this.valuation().date);
  }
  changeDate() {
    this.pricing.clear();
    if (this.appliedDate()) this.model.update((model) => ({ ...model, time_to_expiry: '' }));
    this.appliedDate.set('');
  }
  editScenario(event: Event) {
    this.pricing.clear();
    const id = (event.target as HTMLElement).id;
    if (id === 'pricing-risk_free_rate') this.copiedRate.set('');
    if (id === 'pricing-spot') this.copiedSpot.set('');
    if (id === 'pricing-time_to_expiry') this.appliedDate.set('');
  }

  calculate() {
    if (this.pricing.request().isLoading) return;
    return submit(this.scenarioForm, async () => {
      this.pricing.calculate(this.model());
    });
  }
  reset() {
    this.pricing.clear();
    const contract = this.contract();
    this.model.set(
      contract
        ? {
            exercise_style: 'european',
            right_type: contract.right_type,
            strike: contract.strike,
            spot: '',
            time_to_expiry: '',
            volatility: '',
            risk_free_rate: '',
            dividend_yield: '',
          }
        : sampleScenario(),
    );
    this.valuation.set({ date: '' });
    this.valuationForm().reset();
    this.copiedSpot.set('');
    this.copiedRate.set('');
    this.appliedDate.set('');
    this.scenarioForm().reset();
  }
}
