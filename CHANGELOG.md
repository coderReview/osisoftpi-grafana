# Changelog

## 1.0.0

- Initial release.

## 2.0.0

- Move to React based framework.

## 3.1.0

- Added calculation to PI Points
- Added PI point configuration (thanks to @TheFern2)
- Added option to use last value from PiWebAPI
- Updated to Grafana plugin SDK v9.3.6

## 4.0.0

- Added a new dataframe label format. It can be disabled in the configuration page for backward compatibility
- Added engineering units to Dataframe field. This can be globaly disabled in the configuration page
- Optimized queries using PIWebAPI batch endpoint
- Improved raw query processing
- Added variable support in raw query
- Fixed annotations support
- Updated to Grafana plugin SDK v9.4.7
- Fixed PI AF calculation
- Added plugin screenshots

## 4.1.0

- Modified the PI Webapi controller endpoints used when calculation is selected
- Allow calculation when last value option is selected
- When calculation is selected, change label from Interpolated to Interval
- Fixed issue with variable in Element Path

## 4.2.0

- Fixed issue that only odd attributes were been shown
- Fixed issue when fetching afServerWebId

## 5.0.0

- Migrated backend to Go language
- Changed the query editor layout
- Support Grafana version 11
- Drop support for Grafana 8.x and 9.x

## 5.1.0

- Add units and description to new format - issue #154
- Fixed digital state - issue #159
- Fixed summary data - issue #160
- Fixed an error in recorded max number of points - issue #162
- Fix issue with summary when migrating from previous versions - issue $160

- Updated the query editor layout
- Added boundary type support in recorded values
- Recognize partial usage of variables in elements
- Added configuration to hide API errors in panel
- Truncate time from grafana date time picker to seconds
- Fixed warnings during deploy
- Fixed LICENSE file

### 5.2.0

- Improved query performance to PiWebAPI by joing all queries in Panel into one batch request only
- Change the Query Editor layout
- Increased WebID cache from 1 hour to 12 hours and made it configurable

- Added experimental feature to cache latest response in case of request failure to PiWebAPI

## 6.0.0

- Rebuilt the plugin with current Grafana tooling to fix it failing to load on Grafana 12.3 and later - issue #197
- Fixed units from PI not being added to data frames when "Enable Unit From Data" and "Use unit from datapoints" are enabled - issue #208
- Minimum supported Grafana version is now 11.6.0; tested against Grafana 11.6, 12.x and 13.x
- Updated plugin scaffolding to `@grafana/create-plugin` 7.11 (dynamic public path, subresource integrity, ESLint 9)
- Updated frontend packages to `@grafana/*` 12.x
- Updated backend to `grafana-plugin-sdk-go` v0.296.5 (requires Go 1.26)
- Replaced deprecated `LegacyForms` fields in the configuration page
- Replaced deprecated `DataSourceHttpSettings` with `@grafana/plugin-ui` connection, authentication and advanced HTTP settings
- Replaced deprecated `AsyncSelect` with `Combobox` in the annotations editor
- Removed the committed `dist` build output from the repository
- Added Playwright end-to-end tests that run in CI against every supported Grafana version
