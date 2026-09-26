import { firstVariableValue, formatVariableValue } from './helper';

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
