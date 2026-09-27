import {
  buildQueryString,
  firstVariableValue,
  formatVariableValue,
  migrateLegacyQuery,
  removeServerPrefix,
} from './helper';
import { PIWebAPIQuery } from './types';

describe('formatVariableValue', () => {
  it('keeps single values unchanged', () => {
    expect(formatVariableValue('SiteA')).toBe('SiteA');
    expect(formatVariableValue(['SiteA'])).toBe('SiteA');
    expect(formatVariableValue(42)).toBe('42');
    expect(formatVariableValue(undefined)).toBe('');
  });

  it('formats multiple values as a group', () => {
    expect(formatVariableValue(['SiteA', 'SiteB'])).toBe('{SiteA,SiteB}');
  });

  it('encodes characters that would break the group', () => {
    expect(formatVariableValue(['Pump 1, North', '50% {max}'])).toBe('{Pump 1%2C North,50%25 %7Bmax%7D}');
  });
});

describe('firstVariableValue', () => {
  it('uses the first value of every group', () => {
    expect(firstVariableValue('\\\\AF\\DB\\{S1,S2}\\{U1,U2}')).toBe('\\\\AF\\DB\\S1\\U1');
  });

  it('decodes the first value', () => {
    expect(firstVariableValue(formatVariableValue(['Pump 1, North', 'Pump 2']))).toBe('Pump 1, North');
  });

  it('keeps single-value groups intact', () => {
    expect(firstVariableValue('{SiteA}')).toBe('SiteA');
  });

  it('keeps paths without groups', () => {
    expect(firstVariableValue('\\\\AF\\DB\\Site')).toBe('\\\\AF\\DB\\Site');
  });
});

describe('buildQueryString', () => {
  it('encodes AF paths so they are not cut at # or &', () => {
    const qs = buildQueryString({ path: '\\\\PIServer\\AFName\\Location\\Machine#1&2 + 50%' });
    expect(qs).toBe('?path=%5C%5CPIServer%5CAFName%5CLocation%5CMachine%231%262%20%2B%2050%25');
    expect(new URLSearchParams(qs).get('path')).toBe('\\\\PIServer\\AFName\\Location\\Machine#1&2 + 50%');
  });

  it('skips empty values', () => {
    expect(buildQueryString({ nameFilter: undefined, maxCount: 100, selectedFields: '' })).toBe('?maxCount=100');
    expect(buildQueryString({})).toBe('');
  });
});

describe('removeServerPrefix', () => {
  it('removes the leading backslashes of a UNC-style target', () => {
    expect(removeServerPrefix('\\\\AFSIM\\DB\\E;Level')).toBe('AFSIM\\DB\\E;Level');
    expect(removeServerPrefix('\\\\PISIM;T-101.Level')).toBe('PISIM;T-101.Level');
  });

  it('keeps targets without the prefix', () => {
    expect(removeServerPrefix('AFSIM\\DB\\E;Level')).toBe('AFSIM\\DB\\E;Level');
    expect(removeServerPrefix('')).toBe('');
  });
});

describe('migrateLegacyQuery', () => {
  const average = { label: 'Average', value: { value: 'Average', expandable: true } };
  const legacy = (summary: object, extra: object = {}) =>
    ({ refId: 'A', target: 'AF\\DB\\E;Level', summary, ...extra }) as unknown as PIWebAPIQuery;

  it('enables the summary of a 4.x query with summary types, with its interval and bad data replacement', () => {
    const query = legacy({ types: [average], basis: 'TimeWeighted', interval: ' 1h ', nodata: 'Previous' });
    expect(migrateLegacyQuery(query)).toEqual({
      refId: 'A',
      target: 'AF\\DB\\E;Level',
      nodata: 'Previous',
      summary: { types: [average], basis: 'TimeWeighted', duration: '1h', enable: true },
    });
  });

  it('enables the summary of a 4.x query re-saved by 5.1 or 5.2', () => {
    const query = legacy({
      enable: false,
      duration: '',
      types: [average],
      basis: 'EventWeighted',
      interval: '30m',
      nodata: 'Drop',
    });
    const migrated = migrateLegacyQuery(query);
    expect(migrated.summary).toEqual({ enable: true, duration: '30m', types: [average], basis: 'EventWeighted' });
    expect(migrated.nodata).toBe('Drop');
  });

  it('keeps the summary disabled without summary types and keeps a newer bad data replacement', () => {
    const migrated = migrateLegacyQuery(legacy({ types: [], interval: '', nodata: 'Zero' }, { nodata: 'Previous' }));
    expect(migrated.summary).toEqual({ types: [], enable: false });
    expect(migrated.nodata).toBe('Previous');
  });

  it('replaces the Null bad data replacement written by the query editor', () => {
    const migrated = migrateLegacyQuery(legacy({ types: [], interval: '', nodata: 'Previous' }, { nodata: 'Null' }));
    expect(migrated.nodata).toBe('Previous');
  });

  it('returns current queries unchanged', () => {
    const query = legacy({ enable: false, duration: '1h', types: [average] }, { nodata: 'Null' });
    expect(migrateLegacyQuery(query)).toBe(query);
    const migrated = migrateLegacyQuery(legacy({ types: [average], interval: '1h' }));
    expect(migrateLegacyQuery(migrated)).toBe(migrated);
  });
});
