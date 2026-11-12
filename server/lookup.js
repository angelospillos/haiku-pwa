async function attempt(task) {
  try {
    return { ok: true, candidates: await task() };
  } catch (error) {
    return { ok: false, candidates: [], message: error.message };
  }
}

function describe(outcome) {
  return outcome.ok
    ? { ok: true, count: outcome.candidates.length }
    : { ok: false, count: 0, message: outcome.message };
}

export async function lookupIssue({ series, number, year, comicsOrg, inducks }) {
  const [comicsOrgOutcome, inducksOutcome] = await Promise.all([
    attempt(() => comicsOrg.searchIssues({ series, number, year })),
    attempt(() =>
      inducks
        .searchIssues({ series, number })
        .filter((candidate) => !year || !candidate.publicationDate || candidate.publicationDate.startsWith(year)),
    ),
  ]);

  return {
    series,
    number,
    year: year || null,
    candidates: [...comicsOrgOutcome.candidates, ...inducksOutcome.candidates],
    sources: {
      'comics.org': describe(comicsOrgOutcome),
      inducks: describe(inducksOutcome),
    },
  };
}
