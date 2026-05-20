import http from 'k6/http';
import { check } from 'k6';

export const options = {
  vus: 1,
  duration: '10s',
};

export default function () {
  const payload = JSON.stringify({
    transaction_id: `test_${__VU}_${__ITER}`,
    merchant_id: 'test',
    amount: 100.0,
    time: '2024-01-01T00:00:00Z'
  });
  
  const params = {
    headers: {
      'Content-Type': 'application/json',
    },
  };
  
  const res = http.post('http://localhost:9999/fraud-score', payload, params);
  check(res, {
    'status is 200': (r) => r.status === 200,
  });
}
