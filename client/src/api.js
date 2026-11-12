const CONNECTION_ERROR = "Can't reach the collection server. Check the connection and try again.";

function query(params) {
  return new URLSearchParams(Object.entries(params).filter(([, value]) => value)).toString();
}

async function request(path, options = {}) {
  let response;
  try {
    response = await fetch(path, {
      ...options,
      headers: { Accept: 'application/json', ...(options.body ? { 'Content-Type': 'application/json' } : {}) },
    });
  } catch {
    throw new Error(CONNECTION_ERROR);
  }

  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    const error = new Error(body.error || `Request failed (HTTP ${response.status})`);
    error.status = response.status;
    error.item = body.item;
    throw error;
  }
  return body;
}

export const api = {
  list() {
    return request('/api/collection');
  },

  check(series, number) {
    return request(`/api/collection/check?${query({ series, number })}`);
  },

  lookup(series, number, year) {
    return request(`/api/lookup?${query({ series, number, year })}`);
  },

  add(entry) {
    return request('/api/collection', { method: 'POST', body: JSON.stringify(entry) });
  },

  remove(id) {
    return request(`/api/comics/${id}`, { method: 'DELETE' });
  },
};
