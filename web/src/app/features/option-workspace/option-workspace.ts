import { ChangeDetectionStrategy, Component, inject, OnInit, viewChild } from '@angular/core';
import { EuropeanPricer } from '../european-pricer/european-pricer';
import { DatePipe } from '@angular/common';
import { InstrumentStore } from '../../state/instrument.store';
import { OptionWorkspaceStore } from '../../state/option-workspace.store';
import { Instrument } from '../../core/models';
@Component({
 selector: 'app-option-workspace', imports: [DatePipe, EuropeanPricer],
 providers: [InstrumentStore, OptionWorkspaceStore],
 templateUrl: './option-workspace.html', changeDetection: ChangeDetectionStrategy.OnPush,
})
export class OptionWorkspace implements OnInit {
 private readonly pricer = viewChild(EuropeanPricer);
 useContract(id: string) { this.workspace.selectContract(id); this.pricer()?.focus(); }
 readonly instruments = inject(InstrumentStore);
 readonly workspace = inject(OptionWorkspaceStore);
 ngOnInit() { this.instruments.search('AAPL'); }
 select(instrument: Instrument) { this.instruments.select(instrument); this.workspace.select(instrument); }
 refresh() { const instrument = this.instruments.selected(); if (instrument) this.workspace.select(instrument); }
}
