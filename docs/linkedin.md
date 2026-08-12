# What LinkedIn actually does

Everything here was read off a live session, not inferred. LinkedIn is a moving target: query ids
are build artefacts and the page markup is theirs to change, so when something stops working this is
the first file to re-check against a real browser.

## The calls the extension makes

| Call | Path |
|---|---|
| job search | `GET /voyager/api/voyagerJobsDashJobCards?q=jobSearch` |
| one posting | `GET /voyager/api/jobs/jobPostings/:id` |
| place lookup | `GET /voyager/api/graphql` with the typeahead query id |

Every call carries the person's own session: the csrf token is the `JSESSIONID` cookie value,
`accept` is `application/vnd.linkedin.normalized+json+2.1` and the rest.li protocol version is
`2.0.0`. The API never makes these calls and holds no LinkedIn credential.

The search query is rest.li, not url-encoded json, so its parens, colons and commas are literal:

```
(origin:JOB_SEARCH_PAGE_OTHER_ENTRY,keywords:java,locationUnion:(geoId:102105699),
 selectedFilters:(sortBy:List(DD),timePostedRange:List(r604800),applyWithLinkedin:List(true)),
 spellCorrectionEnabled:true)
```

A keyword narrows hard: one Istanbul week window returned 1929 postings with no keyword and 129 with
`java spring`. It stays loose all the same, a testing role came back among them, which is why the
screener judges everything the query returns instead of trusting the term.

## Places

The place lookup lives at `/voyager/api/graphql` with
`queryId=voyagerSearchDashReusableTypeahead.4c7caa85341b17b470153ad3d1a29caf` and

```
variables=(keywords:<name>,query:(typeaheadFilterQuery:(geoSearchTypes:List(POSTCODE_1,POSTCODE_2,
POPULATED_PLACE,ADMIN_DIVISION_1,ADMIN_DIVISION_2,COUNTRY_REGION,MARKET_AREA,COUNTRY_CLUSTER)),
typeaheadUseCase:JOBS),type:GEO)
```

The older `/voyager/api/typeahead/hitsV2` path answers 404. When it did, every query naming a place
resolved nothing and never fetched a page, while the one worldwide query filled the whole round, and
nothing said so out loud. A place that stops resolving is the first thing to check when rounds go
thin.

## The application form

The job page draws itself inside an open shadow root on a `div.theme--light`. In the light dom there
is no `input[type="file"]` at all, `.artdeco-modal` matches nothing, and the only `[role="dialog"]`
elements are two hidden video.js dialogs. Anything that reads the apply form has to walk open shadow
roots from `document` down, and gate on the resume field itself rather than on a dialog selector.

## Whether a posting was applied to

`jobPostings/:id` answers in normalized json, so `applyingInfo` is a reference on `data` and the
record itself sits in `included`:

```json
{ "$type": "com.linkedin.voyager.entities.shared.JobApplyingInfo",
  "applied": true, "appliedAt": 1786526378000, "closed": false,
  "resumeFileName": "my-cv.pdf" }
```

`applied` and `closed` are direct fields on that node, not nested under `applyingInfo`. This is the
only honest source for whether an application happened: it is true no matter where the person
applied from, including a phone, and it stays true after the tab is closed.

## The model, not LinkedIn

Strict tool schemas make the planner answer with nothing. Measured on the real planning prompt: with
`strict: true` on the plan tool, 1 of 6 calls produced queries and the rest returned an empty plan in
about 150 output tokens; without it, 9 of 10 produced a full plan of 8 queries in about 1800 tokens.
Raising the effort did not help. So no tool is sent strict, and a malformed answer is caught by
validation instead: the query is rejected, kept in the round as one that never ran, and read back by
the next plan.
