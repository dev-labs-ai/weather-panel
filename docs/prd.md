# Product Requirements Document (PRD)

## Overview

The weather panel will let any user check the current weather conditions for a city on a single screen. The user will
enter the city name, and the system will resolve the location and present temperature, feels-like temperature, weather
condition, humidity, and wind in Brazilian Portuguese and in metric units.
The system will query the Open-Meteo geocoding and forecast APIs. When more than one location shares the same name, the
system will use the first result returned and will show the resolved location to give the user context.

## Goals

- Allow the user to complete a current-weather lookup by entering only a city name.
- Present, in 100% of successfully completed lookups, the resolved location, the temperature, the feels-like
  temperature, the weather condition, the humidity, and the wind.
- Complete at least 95% of valid lookups within 3 seconds, measured from submitting the search to displaying the result,
  over a normal connection and with external dependencies available.
- Display clear guidance in 100% of the anticipated scenarios of invalid input, city not found, and external
  unavailability.
- Ensure that, in 100% of lookups, the user's browser sends the search only to the application itself and never
  directly to Open-Meteo or any other third-party service.
- Provide a usable experience on screens 360 px wide and up, operable by keyboard and compatible with assistive
  technologies in the main flows.

## User stories

- US1: As a user, I want to search for a city by name to quickly check the current weather for that location.
- US2: As a user, I want to see the name of the city, region, and country actually selected so I know which location
  the displayed data belongs to.
- US3: As a user, I want to view temperature, feels-like temperature, weather condition, humidity, and wind to
  understand the current conditions.
- US4: As a user, I want to receive clear messages when the search is incomplete, the city is not found, or the service
  is unavailable so I know how to proceed.
- US5: As a keyboard or assistive technology user, I want to fill in, submit, and understand the search and its result
  without relying on a mouse, color, or purely visual elements.
- US6: As a user, I want to identify the source of the weather data to understand where the displayed information comes
  from.

## Key features

### City search

The panel will provide a text field and a search action. The input must accept city names in different languages,
ignore extra whitespace, and require at least two meaningful characters.

- FR1: The system must allow submitting a search by city name.
- FR2: The system must reject inputs that are empty, consist only of whitespace, or have fewer than two meaningful
  characters, and guide the user to correct the search.
- FR3: The system must use the first result returned by the Open-Meteo location search, without presenting a step to
  choose between cities with the same name.
- FR4: The system must show the city, the available administrative division, and the country of the resolved location.

### Display of current conditions

The result must prioritize quick reading and use Brazilian Portuguese language and metric units.

- FR5: The panel must display temperature and feels-like temperature in degrees Celsius.
- FR6: The panel must display the current weather condition as an understandable description in Brazilian Portuguese.
- FR7: The panel must display relative humidity as a percentage.
- FR8: The panel must display wind speed in kilometers per hour.

### States and failure recovery

The panel will make it explicit when a lookup is in progress, has no result, or could not be completed.

- FR9: The system must present a loading state during the lookup and prevent accidental duplicate submissions while it
  is in progress.
- FR10: The system must distinguish invalid input, city not found, and temporary service unavailability through clear
  messages.
- FR11: After a failure, the system must preserve the ability to edit the city and retry the lookup.
- FR12: A previous result must not be presented as if it corresponded to a new search that ended in an error.

### Source transparency

The origin of the data must remain visible alongside the panel.

- FR13: The panel must display a visible attribution to Open-Meteo, with a link to the source, close to the weather
  data.

### Privacy

Lookups must not expose the user's IP address or browser data to third parties together with their searches.

- FR14: The application must obtain the weather data on the user's behalf; the user's browser must send lookups only to
  the application itself, never directly to Open-Meteo or any other third-party service.

## Acceptance criteria

- AC-01 (US1, FR1, FR3): Given a valid city with a result in Open-Meteo, when the user submits the search, then the
  system must display the current conditions for the first location returned.
- AC-02 (US2, FR3, FR4): Given a name associated with multiple locations, when the lookup completes, then no
  intermediate selection must be requested, and the city, the available administrative division, and the country of the
  first result must be identified on the panel.
- AC-03 (US3, FR5–FR8): Given a successfully completed lookup, when the panel presents the result, then it must display
  temperature, feels-like temperature, condition, humidity, and wind with labels in Brazilian Portuguese and metric
  units.
- AC-04 (US1, FR14): Given any search started by the user, when the browser's network requests are inspected, then the
  lookup must be sent only to the application itself, and the browser must not send it directly to the Open-Meteo
  domains or any other third-party service.
- AC-05 (US4, FR2): Given an input that is empty, consists only of whitespace, or has fewer than two meaningful
  characters, when the user tries to search, then they must receive validation guidance and no weather result must be
  presented.
- AC-06 (US4, FR10): Given a search with no matching locations, when it completes, then the panel must state that the
  city was not found and allow a new attempt.
- AC-07 (US4, FR10–FR12): Given an outage or invalid response from an external dependency, when the lookup fails, then
  the panel must present a temporary error message, must not associate a previous result with the new search, and must
  allow retrying.
- AC-08 (US4, FR9): Given a lookup in progress, when the user waits for the response, then they must perceive a loading
  state, and the submit action must not generate accidental duplicate lookups.
- AC-09 (Performance goal): Given a representative set of valid lookups and the external dependencies available, when
  the end-to-end time is measured over a normal connection, then at least 95% of the lookups must display the result
  within 3 seconds.
- AC-10 (US5): Given keyboard-only use, when the user navigates the panel, fills in the city, starts the search, and
  accesses the result or an error message, then all of these actions and information must be available in a logical
  focus order and with a visible focus indicator.
- AC-11 (US5): Given the use of assistive technology, when the loading, success, validation, or error states change,
  then the relevant labels and messages must be identifiable and announced without relying solely on color or icons.
- AC-12 (US5): Given screen widths of 360 px and 1280 px, when the panel is displayed, then its content must remain
  readable and operable, with no horizontal scrolling caused by the feature.
- AC-13 (US6, FR13): Given a visible weather result, when the user views the panel, then they must find an attribution
  to Open-Meteo with a working link alongside the data.

## User experience

The primary audience is anyone who wants to quickly check the current weather for a city. People who use keyboards,
screen readers, or magnification are also part of the audience and must be able to complete the same main flow.

When accessing the panel, the user will find a heading that explains the purpose of the area, a field with a persistent
label for the city name, and a clear search action. After submission, the panel will indicate loading and then replace
that state with the result or an actionable message. The field will remain available so another city can be looked up.

The result will highlight the temperature and the current condition, followed by the remaining data, without relying
solely on icons or colors. The resolved location will be presented explicitly because ambiguous searches will
automatically use the first result. The interface will use Brazilian Portuguese, degrees Celsius, percentage, and
kilometers per hour.

The experience must be responsive from 360 px, maintain AA-level contrast for text and controls, keep focus visible and
navigation order logical, associate programmatic labels with controls, and communicate asynchronous changes to
assistive technologies. Validation and error messages must explain the problem and the possible action, without
internal system terms.

## High-level technical constraints

- City resolution must use the [Open-Meteo Geocoding API](https://geocoding-api.open-meteo.com/v1/search), and current
  conditions must use the [Open-Meteo Weather Forecast API](https://api.open-meteo.com/v1/forecast), both over HTTPS.
- The performance target is up to 3 seconds for at least 95% of valid lookups, measured end to end over a normal
  connection and with Open-Meteo available.
- The MVP assumes non-commercial use of the free API. The provider's current limits must be respected; commercial use or
  volume above the allowed limit will require reassessing the service plan.
- The display of the data must comply with the CC BY 4.0 license and keep the required attribution to Open-Meteo
  alongside the panel.
- The search must not require an account, a user API key, or storage of personal data. The searched name must not be
  persisted as product history. Provider responses may be kept for a limited time to speed up later lookups, provided
  they are not linked to any user, session, or IP address and are never presented as a search history.
- The availability and accuracy of the data depend on Open-Meteo; the product must communicate failures without
  promising continuity or absolute accuracy of the external service.

## Out of scope

- Browser geolocation, automatic retrieval of coordinates, and automatic city suggestions.
- Manual selection between cities with the same name; the MVP will always use the first geocoding result.
- Hourly or daily future forecasts, weather history, and comparison between locations.
- Severe weather alerts, notifications, weather maps, or radar.
- Favorites, search history, user accounts, synchronization, or persistent personalization.
- Switching between metric and imperial units, or support for languages other than Brazilian Portuguese.
- Offline operation, any guarantee of our own for data availability, or automatic replacement of Open-Meteo with another
  provider.
- Purchasing or configuring a commercial Open-Meteo plan for the MVP.
