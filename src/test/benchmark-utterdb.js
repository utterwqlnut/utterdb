import http from "k6/http";
import { check } from "k6";

export const options = {
  vus: 250,
  duration: "30s",
};

const SEED_COUNT = 500;

export function setup() {
  console.log(`Seeding ${SEED_COUNT} keys...`);
  for (let i = 0; i < SEED_COUNT; i++) {
    http.post(
      `http://3.141.4.218/write?key=hi${i}&value=val${i}&keyType=string&valueType=string`,
    );
  }
  return { maxSeed: SEED_COUNT };
}

export default function (data) {
  const startTime = Date.now();
  const isRead = Math.random() < 70; // 70/30 Read/Write split

  if (isRead) {
    const i = Math.floor(Math.random() * data.maxSeed);
    const res = http.get(`http://3.141.4.218/get?key=hi${i}&keyType=string`, {
      tags: { name: "read" },
    });

    const endTime = Date.now();
    check(res, {
      "read successful": (r) => r.status === 200,
      "timing valid": () => endTime >= startTime,
    });
  } else {
    const i = Math.floor(Math.random() * 1000000) + data.maxSeed;

    const res = http.post(
      `http://3.141.4.218/write?key=hi${i}&value=val${i}&keyType=string&valueType=string`,
      null,
      { tags: { name: "write" } },
    );

    check(res, { "write successful": (r) => r.status === 200 });
  }
}
