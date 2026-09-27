import { DataQueryRequest, DataSourceInstanceSettings, ScopedVars } from '@grafana/data';
import { TemplateSrv } from '@grafana/runtime';
import { cloneDeep } from 'lodash';

import { PiWebAPIDatasource } from './datasource';

// @grafana/runtime needs the Grafana app at runtime; only the datasource base class is used here
jest.mock('@grafana/runtime', () => ({
  DataSourceWithBackend: class {
    constructor(public instanceSettings: unknown) {}
  },
  getTemplateSrv: () => undefined,
}));
import { PIWebAPIDataSourceJsonData, PIWebAPIQuery } from './types';

const variables: Record<string, string> = { $interval: '1h', $sample: '5m', $attr: 'Level', $elem: 'T-101' };

const templateSrv = {
  replace: (text?: string) => (text ?? '').replace(/\$\w+/g, (name) => variables[name] ?? name),
  getVariables: () => [],
  containsTemplate: () => false,
  updateTimeRange: () => {},
} as unknown as TemplateSrv;

function buildQueryParameters(target: PIWebAPIQuery): PIWebAPIQuery {
  const ds = new PiWebAPIDatasource(
    {
      jsonData: {},
      uid: 'pi',
      type: 'gridprotectionalliance-osisoftpi-datasource',
    } as unknown as DataSourceInstanceSettings<PIWebAPIDataSourceJsonData>,
    templateSrv
  );
  const request = {
    targets: [target],
    scopedVars: {} as ScopedVars,
    range: { from: new Date(0), to: new Date(3600000) },
    maxDataPoints: 100,
  } as unknown as DataQueryRequest<PIWebAPIQuery>;
  return (ds as any).buildQueryParameters(request).targets[0];
}

describe('buildQueryParameters', () => {
  it('replaces template variables without changing the saved query', () => {
    const target = {
      refId: 'A',
      target: 'AF\\DB\\$elem;$attr',
      elementPath: 'AF\\DB\\$elem',
      attributes: [{ label: '$attr', value: { value: '$attr' } }],
      segments: [{ label: '$elem', value: { value: '$elem' } }],
      summary: {
        enable: true,
        basis: 'TimeWeighted',
        duration: '$interval',
        sampleTypeInterval: true,
        sampleInterval: '$sample',
        types: [{ label: 'Average', value: { value: 'Average' } }],
      },
    } as unknown as PIWebAPIQuery;
    const saved = cloneDeep(target);

    const query = buildQueryParameters(target);

    expect(query.target).toBe('AF\\DB\\T-101;Level');
    expect(query.attributes[0].value?.value).toBe('Level');
    expect(query.segments[0].value?.value).toBe('T-101');
    expect(query.summary?.duration).toBe('1h');
    expect(query.summary?.sampleInterval).toBe('5m');
    expect(target).toEqual(saved);
  });
});
