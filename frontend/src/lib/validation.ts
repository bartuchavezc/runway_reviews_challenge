// Apple's App Store Connect customer-reviews RSS URL pattern. Used on the
// frontend to give the user immediate feedback before the network round-trip;
// the backend re-validates with the same shape (defence in depth).
//
//   https://itunes.apple.com/{country}/rss/customerreviews/id={appId}/...
//
// Anchored at the start so http:// is rejected; trailing slash so the
// captured appId can't be polluted by extra digits from the rest of the
// path. No `$` anchor — Apple's URL has more segments (/sortBy=…/json) that
// we don't constrain here.
export const RSS_REGEX = /^https:\/\/itunes\.apple\.com\/[a-z]{2}\/rss\/customerreviews\/id=(\d+)\//
