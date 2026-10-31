import type { Subject, Notice, Lecture, WeekSchedule, ContestSummaryItem, ContestRecord, FeeItem } from '../types';

export const noticesData: Notice[] = [
  { 
    id: 'notice-1',
    tag: 'ACADEMIC', 
    tagBg: 'bg-claude-terracotta/15 text-claude-terracotta', 
    time: '19 Oct 2026, 2:30 PM', 
    title: 'Mid-Sem Hackathon 2026 Registrations', 
    desc: 'Team registration deadline: 21 October 2026. Contact the academic office for event details.',
    actionLabel: 'View Schedule',
    actionRoute: '/schedule'
  },
  { 
    id: 'notice-2',
    tag: 'ACADEMIC', 
    tagBg: 'bg-duo-blue/15 text-duo-blueDark dark:text-duo-blue', 
    time: '18 Oct 2026, 4:15 PM', 
    title: 'System Design Guest Lecture', 
    desc: 'Principal Architect session in Auditorium B on 23 October 2026 at 4:00 PM for Semester 3.',
    actionLabel: 'Open Subject',
    actionRoute: '/subjects/SD01B1'
  },
  { 
    id: 'notice-3',
    tag: 'CAMPUS', 
    tagBg: 'bg-duo-green/15 text-duo-greenDark dark:text-duo-green', 
    time: '17 Oct 2026', 
    title: '24/7 Reading Room Access', 
    desc: 'Student ID card required for late-night entry after 10:00 PM in the central library.' 
  }
];

export const drawerNotices: Notice[] = [
  {
    id: 'dn-1',
    tag: 'ACADEMIC',
    tagBg: 'bg-claude-terracotta/15 text-claude-terracotta',
    time: '19 Oct 2026, 2:30 PM',
    title: 'Mid-semester hackathon registration',
    desc: 'Team registration deadline: 21 October 2026. Contact the academic office for event details. Problem statements cover Distributed Systems and Applied AI.',
    
  },
  {
    id: 'dn-2',
    tag: 'ACADEMIC',
    tagBg: 'bg-duo-blue/15 text-duo-blueDark dark:text-duo-blue',
    time: '18 Oct 2026, 4:15 PM',
    title: 'Guest lecture: system design',
    desc: 'Principal Architect session in Auditorium B on 23 October 2026, 4:00–5:30 PM. Open System Design-I for course materials.',
    actionLabel: 'Go to System Design-I',
    actionRoute: '/subjects/SD01B1'
  },
  {
    id: 'dn-3',
    tag: 'FINANCE',
    tagBg: 'bg-duo-rose/15 text-duo-rose',
    time: '18 Oct 2026, 11:00 AM',
    title: 'Semester 3 tuition reminder',
    desc: 'Review your Semester 3 balance in Fees. Contact the finance office to arrange payment or confirm a bank transfer.',
    actionLabel: 'Review fees',
    actionRoute: '/fees'
  },
  {
    id: 'dn-4',
    tag: 'CAMPUS',
    tagBg: 'bg-duo-green/15 text-duo-greenDark dark:text-duo-green',
    time: '17 Oct 2026',
    title: '24/7 Reading Room Access for Mid-Term Prep',
    desc: 'Student ID card is required for late-night entry after 10:00 PM in the Central Library and Lab Block B.'
  }
];

export const semesterSubjects: Record<number, Subject[]> = {
  1: [
    { code: 'CS101', credits: 4, instructor: 'Prof. Raghavendra Rao', name: 'Introduction to Computer Programming', next: 'Completed: Pointers, Structs & Dynamic Memory', att: '92%', attendedClasses: 33, totalClasses: 36, skips: '+5 skips' },
    { code: 'MA101', credits: 4, instructor: 'Dr. Ananya Chatterjee', name: 'Linear Algebra & Calculus', next: 'Completed: Eigenvalues & Spectral Theorem', att: '88%', attendedClasses: 30, totalClasses: 34, skips: '+3 skips' },
    { code: 'PH101', credits: 4, instructor: 'Dr. Vikramaditya Sen', name: 'Engineering Physics', next: 'Completed: Quantum Mechanics & Solid State Lab', att: '95%', attendedClasses: 36, totalClasses: 38, skips: '+7 skips' },
    { code: 'EN101', credits: 3, instructor: 'Prof. Meera Nair', name: 'Professional Communication', next: 'Completed: Technical Documentation & Rhetoric', att: '90%', attendedClasses: 27, totalClasses: 30, skips: '+4 skips' }
  ],
  2: [
    { code: 'CS201', credits: 4, instructor: 'Prof. Siddharth Verma', name: 'Data Structures & Algorithms-I', next: 'Completed: Balanced BSTs, Heaps & Hashing', att: '89%', attendedClasses: 32, totalClasses: 36, skips: '+4 skips' },
    { code: 'CS202', credits: 4, instructor: 'Dr. Ananya Chatterjee', name: 'Discrete Mathematical Structures', next: 'Completed: Graph Isomorphism & Combinatorics', att: '85%', attendedClasses: 29, totalClasses: 34, skips: '+2 skips' },
    { code: 'EC201', credits: 4, instructor: 'Prof. Karthik Iyer', name: 'Digital Logic & Computer Organization', next: 'Completed: Pipelined RISC-V Datapath', att: '91%', attendedClasses: 31, totalClasses: 34, skips: '+5 skips' },
    { code: 'HM201', credits: 3, instructor: 'Dr. Priya Deshmukh', name: 'Economics for Engineers', next: 'Completed: Game Theory & Market Microstructure', att: '88%', attendedClasses: 23, totalClasses: 26, skips: '+3 skips' }
  ],
  3: [
    { code: 'E0460A', credits: 4, instructor: 'Prof. Siddharth Verma', name: 'Analysis and Design of Algorithms', next: 'Next: Dynamic Programming & Memoization', att: '86%', attendedClasses: 19, totalClasses: 22, skips: '+3 skips' },
    { code: 'X0240A', credits: 4, instructor: 'Dr. Rohini Kulkarni', name: 'Introduction to Machine Learning', next: 'Next: Gradient Descent & Regularization', att: '89%', attendedClasses: 24, totalClasses: 27, skips: '+4 skips' },
    { code: 'E0463A', credits: 4, instructor: 'Prof. Arjun Mehta', name: 'Web Development-II', next: 'Next: React Server Components & Auth', att: '88%', attendedClasses: 21, totalClasses: 24, skips: '+3 skips' },
    { code: 'E0465B', credits: 4, instructor: 'Prof. Devdutt Pattanaik', name: 'First Principles Thinking', next: 'Next: Deconstructing Complex Systems', att: '92%', attendedClasses: 22, totalClasses: 24, skips: '+4 skips' },
    { code: 'DB01B1', credits: 4, instructor: 'Dr. Nandini Krishnan', name: 'Database Management Systems', next: 'Next: Indexing, B+ Trees & Query Optimization', att: '94%', attendedClasses: 17, totalClasses: 18, skips: '+5 skips' },
    { code: 'SD01B1', credits: 4, instructor: 'Prof. Harishankar Menon', name: 'System Design-I', next: 'Next: Load Balancing & Consistent Hashing', att: '100%', attendedClasses: 24, totalClasses: 24, skips: '+6 skips' }
  ]
};

export const dbmsLecturesData: Lecture[] = [
  {
    id: 106,
    code: 'LEC 06',
    date: '14 Oct 2026',
    month: 'October',
    unit: 2,
    title: 'Query Optimization & Cost-Based Execution Plans',
    rawTitle: 'Query Optimization',
    attendance: 'Present',
    duration: '1h 45m',
    summary: 'Relational algebra equivalences, Selinger cost-based optimizer, hash joins vs. sort-merge joins, and interpreting PostgreSQL EXPLAIN ANALYZE plans.',
    slides: 'DBMS_Lec06_Query_Optimization.pdf',
    assignments: [
      { id: 'db-a6', title: 'Lab 04: PostgreSQL Query Plan Tuning & Index Selection', dueDate: '22 Oct 2026', status: 'Pending', url: 'https://vedam.edu/assignments/dbms-lab04' }
    ],
    resources: [
      { title: 'DBMS_Lec06_Query_Optimization.pdf', type: 'Slide Deck', size: '3.4 MB' },
      { title: 'PostgreSQL EXPLAIN ANALYZE Benchmarks.sql', type: 'SQL Script', size: '48 KB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/dbms-106'
  },
  {
    id: 105,
    code: 'LEC 05',
    date: '08 Oct 2026',
    month: 'October',
    unit: 2,
    title: 'Concurrency Control & Two-Phase Locking (2PL)',
    rawTitle: 'Concurrency Control',
    attendance: 'Present',
    duration: '1h 50m',
    summary: 'Strict vs. rigorous 2PL, isolation anomalies (dirty reads, non-repeatable reads, phantoms), MVCC snapshot isolation, and wait-for graph deadlock detection.',
    slides: 'DBMS_Lec05_Concurrency_2PL.pdf',
    assignments: [
      { id: 'db-a5', title: 'Problem Set 3: Conflict Serializability & Precedence Graphs', dueDate: '15 Oct 2026', status: 'Submitted', score: '19/20', url: 'https://vedam.edu/assignments/dbms-ps3' }
    ],
    resources: [
      { title: 'DBMS_Lec05_Concurrency_2PL.pdf', type: 'Slide Deck', size: '2.8 MB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/dbms-105'
  },
  {
    id: 104,
    code: 'LEC 04',
    date: '01 Oct 2026',
    month: 'October',
    unit: 2,
    title: 'Transaction Processing & ACID Guarantees',
    rawTitle: 'Transactions & ACID',
    attendance: 'Awaiting',
    duration: '1h 40m',
    summary: 'Write-ahead logging (WAL), steal/no-force buffer pool policies, shadow paging, and the 3-phase ARIES recovery algorithm (Analysis, Redo, Undo).',
    slides: 'DBMS_Lec04_WAL_ARIES.pdf',
    assignments: [],
    resources: [
      { title: 'DBMS_Lec04_WAL_ARIES.pdf', type: 'Slide Deck', size: '4.1 MB' },
      { title: 'ARIES Recovery Paper Summary Notes.pdf', type: 'Reading', size: '1.2 MB' }
    ],
    recordingUrl: null
  },
  {
    id: 103,
    code: 'LEC 03',
    date: '23 Sep 2026',
    month: 'September',
    unit: 1,
    title: 'B+ Tree Indexes & Storage Engine Architecture',
    rawTitle: 'Indexing & B+ Trees',
    attendance: 'Present',
    duration: '1h 55m',
    summary: 'Slotted-page layout, internal node splitting and merging, clustered vs. unclustered secondary indexes, and covering index range scan mechanics.',
    slides: 'DBMS_Lec03_BPlus_Trees.pdf',
    assignments: [
      { id: 'db-a3', title: 'Lab 03: In-Memory B+ Tree Node Split Implementation', dueDate: '30 Sep 2026', status: 'Graded', score: '50/50', url: 'https://vedam.edu/assignments/dbms-lab03' }
    ],
    resources: [
      { title: 'DBMS_Lec03_BPlus_Trees.pdf', type: 'Slide Deck', size: '3.9 MB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/dbms-103'
  },
  {
    id: 102,
    code: 'LEC 02',
    date: '16 Sep 2026',
    month: 'September',
    unit: 1,
    title: 'Schema Normalization: 1NF through BCNF',
    rawTitle: 'Normalization',
    attendance: 'Present',
    duration: '1h 45m',
    summary: 'Armstrong axioms, canonical cover of functional dependencies, lossless-join decomposition, dependency preservation, and eliminating anomalies.',
    slides: 'DBMS_Lec02_Normalization.pdf',
    assignments: [],
    resources: [
      { title: 'DBMS_Lec02_Normalization.pdf', type: 'Slide Deck', size: '2.2 MB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/dbms-102'
  },
  {
    id: 101,
    code: 'LEC 01',
    date: '09 Sep 2026',
    month: 'September',
    unit: 1,
    title: 'Relational Data Model & Advanced SQL Queries',
    rawTitle: 'Relational Model',
    attendance: 'Present',
    duration: '1h 45m',
    summary: 'Relational algebra primitives, window functions (RANK, LAG, LEAD), recursive CTEs, and referential integrity constraints.',
    slides: 'DBMS_Lec01_Relational_Model.pdf',
    assignments: [
      { id: 'db-a1', title: 'Lab 01: Analytical SQL Window Functions & Recursive CTEs', dueDate: '16 Sep 2026', status: 'Graded', score: '30/30', url: 'https://vedam.edu/assignments/dbms-lab01' }
    ],
    resources: [
      { title: 'DBMS_Lec01_Relational_Model.pdf', type: 'Slide Deck', size: '2.6 MB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/dbms-101'
  }
];

export const systemDesignLecturesData: Lecture[] = [
  {
    id: 12,
    code: 'LEC 12',
    date: '15 Oct 2026',
    month: 'October',
    unit: 2,
    title: 'Load Balancing & Consistent Hashing Rings',
    rawTitle: 'System Design-I',
    attendance: 'Present',
    duration: '1h 50m',
    summary: 'Deep dive into L4 (TCP/UDP) vs. L7 (HTTP) load balancers, consistent hashing rings with virtual nodes, bounded-load hashing, and active health checks.',
    slides: 'SD_Lec12_Consistent_Hashing.pdf',
    assignments: [
      { id: 'sd-a12', title: 'Design Doc 03: Consistent Hashing Ring with Virtual Nodes in Go/TS', dueDate: '23 Oct 2026', status: 'Pending', url: 'https://vedam.edu/assignments/sd-doc03' }
    ],
    resources: [
      { title: 'SD_Lec12_Consistent_Hashing.pdf', type: 'Slide Deck', size: '4.2 MB' },
      { title: 'Dynamo & Karger Consistent Hashing Benchmark.pdf', type: 'Case Study', size: '1.8 MB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/sd-12'
  },
  {
    id: 11,
    code: 'LEC 11',
    date: '10 Oct 2026',
    month: 'October',
    unit: 2,
    title: 'Distributed Caching & Redis Internals',
    rawTitle: 'System Design-I',
    attendance: 'Present',
    duration: '1h 45m',
    summary: 'Write-through, write-back, and cache-aside strategies; LRU/LFU eviction; mitigating cache stampedes (singleflight / probabilistic early expiration) and hot keys.',
    slides: 'SD_Lec11_Distributed_Caching.pdf',
    assignments: [
      { id: 'sd-a11', title: 'Lab 02: Singleflight Stampede Protection Middleware', dueDate: '17 Oct 2026', status: 'Submitted', score: '25/25', url: 'https://vedam.edu/assignments/sd-lab02' }
    ],
    resources: [
      { title: 'SD_Lec11_Distributed_Caching.pdf', type: 'Slide Deck', size: '3.5 MB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/sd-11'
  },
  {
    id: 10,
    code: 'LEC 10',
    date: '09 Oct 2026',
    month: 'October',
    unit: 2,
    title: 'CAP Theorem & PACELC Trade-offs',
    rawTitle: 'System Design-I',
    attendance: 'Present',
    duration: '1h 40m',
    summary: 'Linearizability vs. sequential consistency, quorum reads/writes (R + W > N), sloppy quorums, hinted handoff, and PACELC latency-consistency spectrum.',
    slides: 'SD_Lec10_CAP_PACELC.pdf',
    assignments: [],
    resources: [
      { title: 'SD_Lec10_CAP_PACELC.pdf', type: 'Slide Deck', size: '2.9 MB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/sd-10'
  },
  {
    id: 9,
    code: 'LEC 09',
    date: '28 Sep 2026',
    month: 'September',
    unit: 1,
    title: 'Sharding, Replication & Leader Election',
    rawTitle: 'System Design-I',
    attendance: 'Present',
    duration: '1h 55m',
    summary: 'Range-based vs. hash-based partitioning, single-leader vs. multi-leader replication lag, fencing tokens, and Raft consensus leader election.',
    slides: 'SD_Lec09_Sharding_Replication.pdf',
    assignments: [
      { id: 'sd-a09', title: 'Design Doc 02: Multi-Region Read Replica Routing', dueDate: '05 Oct 2026', status: 'Graded', score: '48/50', url: 'https://vedam.edu/assignments/sd-doc02' }
    ],
    resources: [
      { title: 'SD_Lec09_Sharding_Replication.pdf', type: 'Slide Deck', size: '4.6 MB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/sd-09'
  },
  {
    id: 8,
    code: 'LEC 08',
    date: '19 Sep 2026',
    month: 'September',
    unit: 1,
    title: 'High-Throughput Message Queues & Event Streams',
    rawTitle: 'System Design-I',
    attendance: 'Present',
    duration: '1h 45m',
    summary: 'Asynchronous decoupling with Kafka partitioned append-only logs vs. RabbitMQ AMQP brokers; consumer groups, idempotency, and dead-letter queues.',
    slides: 'SD_Lec08_Message_Queues.pdf',
    assignments: [],
    resources: [
      { title: 'SD_Lec08_Message_Queues.pdf', type: 'Slide Deck', size: '3.1 MB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/sd-08'
  },
  {
    id: 7,
    code: 'LEC 07',
    date: '11 Sep 2026',
    month: 'September',
    unit: 1,
    title: 'Horizontal Scaling & Latency Numbers Every Engineer Should Know',
    rawTitle: 'System Design-I',
    attendance: 'Excused',
    duration: '1h 40m',
    summary: 'Back-of-the-envelope capacity estimation (QPS, bandwidth, storage), tail latency amplification (p95/p99), and stateless tier decomposition.',
    slides: 'SD_Lec07_Scaling_Foundations.pdf',
    assignments: [
      { id: 'sd-a07', title: 'Design Doc 01: Back-of-the-Envelope Estimation for URL Shortener', dueDate: '18 Sep 2026', status: 'Graded', score: '20/20', url: 'https://vedam.edu/assignments/sd-doc01' }
    ],
    resources: [
      { title: 'SD_Lec07_Scaling_Foundations.pdf', type: 'Slide Deck', size: '2.7 MB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/sd-07'
  }
];

export const adaLecturesData: Lecture[] = [
  {
    id: 204,
    code: 'LEC 08',
    date: '16 Oct 2026',
    month: 'October',
    unit: 2,
    title: 'Dynamic Programming: Overlapping Subproblems & Tabulation',
    rawTitle: 'Dynamic Programming',
    attendance: 'Present',
    duration: '1h 50m',
    summary: 'State transition formulation, memoization vs. bottom-up tabulation, 0/1 Knapsack, Longest Common Subsequence, and space-optimized DP.',
    slides: 'ADA_Lec08_Dynamic_Programming.pdf',
    assignments: [
      { id: 'ada-a4', title: 'Contest Set 04: 6 DP Problems on Grid & Subsequences', dueDate: '21 Oct 2026', status: 'Pending', url: 'https://vedam.edu/assignments/ada-set04' }
    ],
    resources: [
      { title: 'ADA_Lec08_Dynamic_Programming.pdf', type: 'Slide Deck', size: '3.2 MB' },
      { title: 'DP State Transition Cheat Sheet.pdf', type: 'Handout', size: '840 KB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/ada-204'
  },
  {
    id: 203,
    code: 'LEC 07',
    date: '12 Oct 2026',
    month: 'October',
    unit: 2,
    title: 'Greedy Exchange Arguments & Matroid Foundations',
    rawTitle: 'Greedy Algorithms',
    attendance: 'Present',
    duration: '1h 45m',
    summary: 'Interval scheduling, Huffman prefix codes, fractional knapsack, and formal exchange argument proofs of global optimality.',
    slides: 'ADA_Lec07_Greedy_Proofs.pdf',
    assignments: [
      { id: 'ada-a3', title: 'Problem Set 03: Greedy Exchange Proofs & Huffman Coding', dueDate: '16 Oct 2026', status: 'Submitted', score: '38/40', url: 'https://vedam.edu/assignments/ada-ps03' }
    ],
    resources: [
      { title: 'ADA_Lec07_Greedy_Proofs.pdf', type: 'Slide Deck', size: '2.5 MB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/ada-203'
  },
  {
    id: 202,
    code: 'LEC 06',
    date: '05 Oct 2026',
    month: 'October',
    unit: 1,
    title: 'Divide and Conquer: Master Theorem & Karatsuba Multiplication',
    rawTitle: 'Divide & Conquer',
    attendance: 'Present',
    duration: '1h 45m',
    summary: 'Solving recurrence relations via recursion trees and the Master Theorem; closest pair of points in O(n log n) and fast polynomial multiplication.',
    slides: 'ADA_Lec06_Divide_Conquer.pdf',
    assignments: [],
    resources: [
      { title: 'ADA_Lec06_Divide_Conquer.pdf', type: 'Slide Deck', size: '2.9 MB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/ada-202'
  },
  {
    id: 201,
    code: 'LEC 05',
    date: '28 Sep 2026',
    month: 'September',
    unit: 1,
    title: 'Shortest Paths: Dijkstra, Bellman-Ford & Floyd-Warshall',
    rawTitle: 'Graph Shortest Paths',
    attendance: 'Awaiting',
    duration: '1h 55m',
    summary: 'Edge relaxation invariants, negative-weight cycle detection with Bellman-Ford, and all-pairs shortest paths via dynamic programming.',
    slides: 'ADA_Lec05_Shortest_Paths.pdf',
    assignments: [
      { id: 'ada-a2', title: 'Contest Set 03: Weighted Graphs & Shortest Path Variants', dueDate: '04 Oct 2026', status: 'Graded', score: '90/100', url: 'https://vedam.edu/assignments/ada-set03' }
    ],
    resources: [
      { title: 'ADA_Lec05_Shortest_Paths.pdf', type: 'Slide Deck', size: '3.7 MB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/ada-201'
  }
];

export const mlLecturesData: Lecture[] = [
  {
    id: 304,
    code: 'LEC 07',
    date: '15 Oct 2026',
    month: 'October',
    unit: 2,
    title: 'Stochastic Gradient Descent, L1/L2 Regularization & Bias-Variance',
    rawTitle: 'Gradient Descent & Regularization',
    attendance: 'Present',
    duration: '1h 50m',
    summary: 'Convex optimization landscapes, mini-batch SGD momentum, Ridge (L2) vs. Lasso (L1) sparsity geometry, and k-fold cross-validation.',
    slides: 'ML_Lec07_Regularization_SGD.pdf',
    assignments: [
      { id: 'ml-a3', title: 'Notebook 03: Implementing Ridge & Lasso Coordinate Descent in NumPy', dueDate: '24 Oct 2026', status: 'Pending', url: 'https://vedam.edu/assignments/ml-nb03' }
    ],
    resources: [
      { title: 'ML_Lec07_Regularization_SGD.pdf', type: 'Slide Deck', size: '4.4 MB' },
      { title: 'regularization_contours_demo.ipynb', type: 'Jupyter Notebook', size: '620 KB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/ml-304'
  },
  {
    id: 303,
    code: 'LEC 06',
    date: '08 Oct 2026',
    month: 'October',
    unit: 2,
    title: 'Logistic Regression, Cross-Entropy Loss & Generalized Linear Models',
    rawTitle: 'Logistic Regression',
    attendance: 'Present',
    duration: '1h 45m',
    summary: 'Maximum likelihood estimation for Bernoulli targets, sigmoid/softmax decision boundaries, ROC-AUC vs. Precision-Recall curves.',
    slides: 'ML_Lec06_Logistic_Regression.pdf',
    assignments: [
      { id: 'ml-a2', title: 'Notebook 02: Credit Default Classifier & ROC Threshold Calibration', dueDate: '15 Oct 2026', status: 'Graded', score: '47/50', url: 'https://vedam.edu/assignments/ml-nb02' }
    ],
    resources: [
      { title: 'ML_Lec06_Logistic_Regression.pdf', type: 'Slide Deck', size: '3.6 MB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/ml-303'
  },
  {
    id: 302,
    code: 'LEC 05',
    date: '01 Oct 2026',
    month: 'October',
    unit: 1,
    title: 'Linear Regression: Normal Equations & Geometric Projection',
    rawTitle: 'Linear Regression',
    attendance: 'Present',
    duration: '1h 45m',
    summary: 'Ordinary least squares as orthogonal projection onto column space, Moore-Penrose pseudoinverse, and Gaussian noise assumptions.',
    slides: 'ML_Lec05_OLS_Geometry.pdf',
    assignments: [],
    resources: [
      { title: 'ML_Lec05_OLS_Geometry.pdf', type: 'Slide Deck', size: '3.1 MB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/ml-302'
  }
];

export const webDevLecturesData: Lecture[] = [
  {
    id: 404,
    code: 'LEC 09',
    date: '16 Oct 2026',
    month: 'October',
    unit: 2,
    title: 'Server Components, Streaming SSR & Session Authentication',
    rawTitle: 'State Management & SSR',
    attendance: 'Present',
    duration: '1h 50m',
    summary: 'React RSC wire format, Suspense streaming boundaries, HTTP-only signed session cookies vs. JWT rotation, and CSRF mitigation.',
    slides: 'Web2_Lec09_SSR_Auth.pdf',
    assignments: [
      { id: 'wd-a4', title: 'Milestone 03: Full-Stack Authenticated Dashboard with Optimistic UI', dueDate: '25 Oct 2026', status: 'Pending', url: 'https://vedam.edu/assignments/web2-m03' }
    ],
    resources: [
      { title: 'Web2_Lec09_SSR_Auth.pdf', type: 'Slide Deck', size: '3.8 MB' },
      { title: 'Auth Session Security Checklist.md', type: 'Guide', size: '24 KB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/web2-404'
  },
  {
    id: 403,
    code: 'LEC 08',
    date: '09 Oct 2026',
    month: 'October',
    unit: 2,
    title: 'Client State Machines, URL Synchronization & Cache Invalidation',
    rawTitle: 'State Management',
    attendance: 'Present',
    duration: '1h 45m',
    summary: 'Derived state vs. synchronized state, stale-while-revalidate caching patterns, optimistic mutations, and race-condition cancellation.',
    slides: 'Web2_Lec08_State_Sync.pdf',
    assignments: [
      { id: 'wd-a3', title: 'Milestone 02: Custom Query Cache & Debounced Search Hook', dueDate: '16 Oct 2026', status: 'Submitted', score: '30/30', url: 'https://vedam.edu/assignments/web2-m02' }
    ],
    resources: [
      { title: 'Web2_Lec08_State_Sync.pdf', type: 'Slide Deck', size: '2.9 MB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/web2-403'
  },
  {
    id: 402,
    code: 'LEC 07',
    date: '02 Oct 2026',
    month: 'October',
    unit: 1,
    title: 'Browser Rendering Pipeline, Layout Thrashing & Compositor Layers',
    rawTitle: 'Rendering Performance',
    attendance: 'Present',
    duration: '1h 40m',
    summary: 'DOM -> Style -> Layout -> Paint -> Composite pipeline, GPU-accelerated transform/opacity animations, and Flamegraph profiling.',
    slides: 'Web2_Lec07_Browser_Pipeline.pdf',
    assignments: [],
    resources: [
      { title: 'Web2_Lec07_Browser_Pipeline.pdf', type: 'Slide Deck', size: '4.0 MB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/web2-402'
  }
];

export const fptLecturesData: Lecture[] = [
  {
    id: 504,
    code: 'LEC 06',
    date: '19 Oct 2026',
    month: 'October',
    unit: 2,
    title: 'Deconstructing Complex Systems: Bottlenecks, Feedback Loops & Leverage Points',
    rawTitle: 'First Principles Thinking',
    attendance: 'Present',
    duration: '1h 45m',
    summary: 'Separating physical constraints from historical conventions; Donella Meadows stocks, balancing vs. reinforcing feedback loops, and system archetypes.',
    slides: 'FPT_Lec06_Complex_Systems.pdf',
    assignments: [
      { id: 'fpt-a3', title: 'Case Memo 02: First-Principles Teardown of Urban Logistics Latency', dueDate: '26 Oct 2026', status: 'Pending', url: 'https://vedam.edu/assignments/fpt-memo02' }
    ],
    resources: [
      { title: 'FPT_Lec06_Complex_Systems.pdf', type: 'Slide Deck', size: '2.7 MB' },
      { title: 'Donella Meadows — Leverage Points Excerpt.pdf', type: 'Reading', size: '1.1 MB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/fpt-504'
  },
  {
    id: 503,
    code: 'LEC 05',
    date: '12 Oct 2026',
    month: 'October',
    unit: 2,
    title: 'Fermi Estimation, Dimensional Analysis & Order-of-Magnitude Reasoning',
    rawTitle: 'Fermi Estimation',
    attendance: 'Present',
    duration: '1h 40m',
    summary: 'Bounding uncertainty with geometric means, decomposing intractable engineering questions into verifiable physical factors.',
    slides: 'FPT_Lec05_Fermi_Estimation.pdf',
    assignments: [
      { id: 'fpt-a2', title: 'Case Memo 01: Datacenter Thermal & Power Budget from First Principles', dueDate: '18 Oct 2026', status: 'Graded', score: '24/25', url: 'https://vedam.edu/assignments/fpt-memo01' }
    ],
    resources: [
      { title: 'FPT_Lec05_Fermi_Estimation.pdf', type: 'Slide Deck', size: '2.1 MB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/fpt-503'
  },
  {
    id: 502,
    code: 'LEC 04',
    date: '05 Oct 2026',
    month: 'October',
    unit: 1,
    title: 'Socratic Questioning, Falsifiability & Cognitive Biases in Engineering',
    rawTitle: 'Falsifiability & Biases',
    attendance: 'Present',
    duration: '1h 45m',
    summary: 'Popperian falsifiability in debugging, survivorship bias in telemetry, sunk-cost fallacy in architecture migrations, and pre-mortems.',
    slides: 'FPT_Lec04_Falsifiability.pdf',
    assignments: [],
    resources: [
      { title: 'FPT_Lec04_Falsifiability.pdf', type: 'Slide Deck', size: '2.4 MB' }
    ],
    recordingUrl: 'https://vedam.edu/recordings/fpt-502'
  }
];

export const archiveSemesterLectures: Record<string, Lecture[]> = {
  CS101: [
    {
      id: 601,
      code: 'LEC 24',
      date: '28 Nov 2025',
      month: 'November',
      unit: 4,
      title: 'Pointers, Dynamic Memory Allocation & Valgrind Profiling',
      rawTitle: 'Pointers & Memory',
      attendance: 'Present',
      duration: '1h 50m',
      summary: 'Stack vs. heap frames, malloc/free lifecycle, dangling pointers, and building a custom arena allocator in C.',
      slides: 'CS101_Lec24_Memory_Allocator.pdf',
      assignments: [{ id: 'cs101-a1', title: 'Final Lab: Custom Arena Allocator in C', dueDate: '05 Dec 2025', status: 'Graded', score: '100/100', url: 'https://vedam.edu/assignments/cs101-final' }],
      resources: [{ title: 'CS101_Lec24_Memory_Allocator.pdf', type: 'Slide Deck', size: '2.9 MB' }],
      recordingUrl: 'https://vedam.edu/recordings/cs101-24'
    }
  ],
  MA101: [
    {
      id: 602,
      code: 'LEC 22',
      date: '25 Nov 2025',
      month: 'November',
      unit: 4,
      title: 'Spectral Theorem, SVD & Principal Component Geometry',
      rawTitle: 'Eigenvalues & SVD',
      attendance: 'Present',
      duration: '1h 45m',
      summary: 'Orthogonal diagonalization of symmetric matrices, Singular Value Decomposition, and low-rank matrix approximation.',
      slides: 'MA101_Lec22_SVD_Spectral.pdf',
      assignments: [{ id: 'ma101-a1', title: 'Problem Set 08: SVD Image Compression', dueDate: '02 Dec 2025', status: 'Graded', score: '48/50', url: 'https://vedam.edu/assignments/ma101-ps8' }],
      resources: [{ title: 'MA101_Lec22_SVD_Spectral.pdf', type: 'Slide Deck', size: '3.1 MB' }],
      recordingUrl: 'https://vedam.edu/recordings/ma101-22'
    }
  ],
  PH101: [
    {
      id: 603,
      code: 'LEC 20',
      date: '24 Nov 2025',
      month: 'November',
      unit: 4,
      title: 'Semiconductor Band Theory & Quantum Tunneling Diodes',
      rawTitle: 'Quantum Mechanics Lab',
      attendance: 'Present',
      duration: '1h 45m',
      summary: 'Schrödinger wave equation in periodic potentials, Fermi-Dirac distribution, and PN-junction carrier transport.',
      slides: 'PH101_Lec20_Semiconductors.pdf',
      assignments: [],
      resources: [{ title: 'PH101_Lec20_Semiconductors.pdf', type: 'Slide Deck', size: '3.5 MB' }],
      recordingUrl: 'https://vedam.edu/recordings/ph101-20'
    }
  ],
  EN101: [
    {
      id: 604,
      code: 'LEC 16',
      date: '21 Nov 2025',
      month: 'November',
      unit: 3,
      title: 'Engineering RFCs, Post-Mortems & Clarity in Technical Prose',
      rawTitle: 'Technical Essay Writing',
      attendance: 'Present',
      duration: '1h 30m',
      summary: 'Structuring unambiguous Request for Comments (RFC) documents, active voice precision, and executive technical summaries.',
      slides: 'EN101_Lec16_RFC_Writing.pdf',
      assignments: [],
      resources: [{ title: 'EN101_Lec16_RFC_Writing.pdf', type: 'Slide Deck', size: '1.8 MB' }],
      recordingUrl: null
    }
  ],
  CS201: [
    {
      id: 701,
      code: 'LEC 26',
      date: '22 Apr 2026',
      month: 'April',
      unit: 4,
      title: 'AVL rotations, Red-Black Trees & Priority Queue Heaps',
      rawTitle: 'Binary Search Trees & Heaps',
      attendance: 'Present',
      duration: '1h 50m',
      summary: 'Self-balancing tree invariants, O(log n) worst-case rotations, binary heap sift-up/sift-down, and amortized analysis.',
      slides: 'CS201_Lec26_Balanced_Trees.pdf',
      assignments: [{ id: 'cs201-a1', title: 'Lab 08: Order-Statistic Tree in C++', dueDate: '29 Apr 2026', status: 'Graded', score: '50/50', url: 'https://vedam.edu/assignments/cs201-lab8' }],
      resources: [{ title: 'CS201_Lec26_Balanced_Trees.pdf', type: 'Slide Deck', size: '3.3 MB' }],
      recordingUrl: 'https://vedam.edu/recordings/cs201-26'
    }
  ],
  CS202: [
    {
      id: 702,
      code: 'LEC 24',
      date: '20 Apr 2026',
      month: 'April',
      unit: 4,
      title: 'Planar Graphs, Euler Formula & Chromatic Polynomials',
      rawTitle: 'Graph Theory & Relations',
      attendance: 'Present',
      duration: '1h 45m',
      summary: 'Kuratowski theorem, bipartite matching (Hall marriage theorem), and graph coloring bounds.',
      slides: 'CS202_Lec24_Planar_Graphs.pdf',
      assignments: [],
      resources: [{ title: 'CS202_Lec24_Planar_Graphs.pdf', type: 'Slide Deck', size: '2.7 MB' }],
      recordingUrl: 'https://vedam.edu/recordings/cs202-24'
    }
  ],
  EC201: [
    {
      id: 703,
      code: 'LEC 22',
      date: '18 Apr 2026',
      month: 'April',
      unit: 4,
      title: '5-Stage Pipelined RISC-V Processor & Hazard Forwarding',
      rawTitle: 'Boolean Algebra & Flip-Flops',
      attendance: 'Present',
      duration: '1h 50m',
      summary: 'Data hazards, ALU forwarding paths, load-use pipeline stalls, and branch prediction buffers in Verilog.',
      slides: 'EC201_Lec22_RISCV_Pipeline.pdf',
      assignments: [],
      resources: [{ title: 'EC201_Lec22_RISCV_Pipeline.pdf', type: 'Slide Deck', size: '4.1 MB' }],
      recordingUrl: 'https://vedam.edu/recordings/ec201-22'
    }
  ],
  HM201: [
    {
      id: 704,
      code: 'LEC 18',
      date: '15 Apr 2026',
      month: 'April',
      unit: 3,
      title: 'Net Present Value, Unit Economics & SaaS Infrastructure Costing',
      rawTitle: 'Cost-Benefit Analysis',
      attendance: 'Present',
      duration: '1h 30m',
      summary: 'Discounted cash flows, cloud compute marginal cost curves, and capital allocation trade-offs.',
      slides: 'HM201_Lec18_Unit_Economics.pdf',
      assignments: [],
      resources: [{ title: 'HM201_Lec18_Unit_Economics.pdf', type: 'Slide Deck', size: '2.0 MB' }],
      recordingUrl: null
    }
  ]
};

export const lecturesData: Lecture[] = [
  ...systemDesignLecturesData,
  ...dbmsLecturesData,
  ...adaLecturesData,
  ...mlLecturesData,
  ...webDevLecturesData,
  ...fptLecturesData
];

export const lecturesBySubject: Record<string, Lecture[]> = {
  DB01B1: dbmsLecturesData,
  SD01B1: systemDesignLecturesData,
  E0460A: adaLecturesData,
  X0240A: mlLecturesData,
  E0463A: webDevLecturesData,
  E0465B: fptLecturesData,
  ...archiveSemesterLectures
};

export const contestSummary: Record<string, ContestSummaryItem> = {
  E0460A: {
    percent: 84,
    detail: 'Recursion, Greedy & Dynamic Programming contests',
    completed: 6,
    total: 6,
    bestRank: '#12 / 180',
    history: [
      { label: 'C1', title: 'Contest #01 — Recursion & Backtracking', date: '05 Sep 2026', score: '312 / 400', percent: 78, rank: '#21 / 180' },
      { label: 'C2', title: 'Contest #02 — Divide & Conquer Recurrences', date: '14 Sep 2026', score: '328 / 400', percent: 82, rank: '#18 / 180' },
      { label: 'C3', title: 'Contest #03 — Weighted Graph Shortest Paths', date: '24 Sep 2026', score: '344 / 400', percent: 86, rank: '#14 / 179' },
      { label: 'C4', title: 'Contest #04 — Minimum Spanning Trees & DSU', date: '03 Oct 2026', score: '324 / 400', percent: 81, rank: '#17 / 180' },
      { label: 'C5', title: 'Contest #05 — Greedy Exchange Proofs & Matroids', date: '10 Oct 2026', score: '368 / 400', percent: 92, rank: '#12 / 180' },
      { label: 'C6', title: 'Contest #06 — Dynamic Programming & Memoization', date: '17 Oct 2026', score: '340 / 400', percent: 85, rank: '#12 / 178' }
    ]
  },
  X0240A: {
    percent: 81,
    detail: 'NumPy vectorization & model evaluation benchmarks',
    completed: 4,
    total: 4,
    bestRank: '#19 / 180',
    history: [
      { label: 'P1', title: 'Practicum #01 — NumPy Vectorization & Broadcasting', date: '12 Sep 2026', score: '76 / 100', percent: 76, rank: '#26 / 180' },
      { label: 'P2', title: 'Practicum #02 — Ordinary Least Squares & Projection', date: '22 Sep 2026', score: '83 / 100', percent: 83, rank: '#20 / 178' },
      { label: 'P3', title: 'Practicum #03 — K-Fold Cross-Validation & Ridge', date: '30 Sep 2026', score: '84 / 100', percent: 84, rank: '#19 / 180' },
      { label: 'P4', title: 'Practicum #04 — Regularized Logistic Regression', date: '08 Oct 2026', score: '81 / 100', percent: 81, rank: '#19 / 175' }
    ]
  },
  E0463A: {
    percent: 76,
    detail: 'DOM architecture, Async JS & SSR state challenges',
    completed: 5,
    total: 5,
    bestRank: '#24 / 180',
    history: [
      { label: 'S1', title: 'Sprint #01 — DOM Event Delegation & Virtualization', date: '08 Sep 2026', score: '72 / 100', percent: 72, rank: '#31 / 180' },
      { label: 'S2', title: 'Sprint #02 — Browser Rendering & Compositor Layers', date: '17 Sep 2026', score: '75 / 100', percent: 75, rank: '#28 / 179' },
      { label: 'S3', title: 'Sprint #03 — Client State Machines & URL Sync', date: '25 Sep 2026', score: '79 / 100', percent: 79, rank: '#24 / 180' },
      { label: 'S4', title: 'Sprint #04 — Async Race Conditions & Caching', date: '04 Oct 2026', score: '76 / 100', percent: 76, rank: '#24 / 179' },
      { label: 'S5', title: 'Sprint #05 — Streaming SSR & Session Auth', date: '13 Oct 2026', score: '78 / 100', percent: 78, rank: '#25 / 180' }
    ]
  },
  DB01B1: {
    percent: 86,
    detail: 'Schema normalization & SQL query optimization challenges',
    completed: 5,
    total: 5,
    bestRank: '#09 / 180',
    history: [
      { label: 'Q1', title: 'Sprint #01 — Analytical Window Functions & CTEs', date: '11 Sep 2026', score: '82 / 100', percent: 82, rank: '#15 / 180' },
      { label: 'Q2', title: 'Sprint #02 — Lossless BCNF Schema Decomposition', date: '19 Sep 2026', score: '85 / 100', percent: 85, rank: '#12 / 178' },
      { label: 'Q3', title: 'Sprint #03 — B+ Tree Page Split & Range Scans', date: '27 Sep 2026', score: '88 / 100', percent: 88, rank: '#10 / 180' },
      { label: 'Q4', title: 'Sprint #04 — MVCC Snapshot Isolation & Deadlocks', date: '06 Oct 2026', score: '83 / 100', percent: 83, rank: '#13 / 177' },
      { label: 'Q5', title: 'Sprint #05 — SQL Query Planner & Indexing', date: '14 Oct 2026', score: '92 / 100', percent: 92, rank: '#09 / 176' }
    ]
  },
  SD01B1: {
    percent: 82,
    detail: 'Distributed architecture & high-availability design reviews',
    completed: 4,
    total: 4,
    bestRank: '#14 / 180',
    history: [
      { label: 'D1', title: 'Review #01 — Back-of-the-Envelope & Tail Latency', date: '13 Sep 2026', score: '39 / 50', percent: 78, rank: '#19 / 180' },
      { label: 'D2', title: 'Review #02 — Kafka Partition Logs & Consumer Groups', date: '23 Sep 2026', score: '42 / 50', percent: 84, rank: '#14 / 179' },
      { label: 'D3', title: 'Review #03 — Raft Consensus & Read Replica Routing', date: '02 Oct 2026', score: '42 / 50', percent: 84, rank: '#15 / 180' },
      { label: 'D4', title: 'Review #04 — Consistent Hashing Ring Architecture', date: '11 Oct 2026', score: '41 / 50', percent: 82, rank: '#14 / 180' }
    ]
  },
  CS201: {
    percent: 85,
    detail: 'Trees, Heaps & Graph traversal timed sprints',
    completed: 4,
    total: 4,
    bestRank: '#11 / 180',
    history: [
      { label: 'C1', title: 'Sprint #01 — Monotonic Stacks & Deques', date: '18 Jan 2026', score: '80 / 100', percent: 80, rank: '#18 / 180' },
      { label: 'C2', title: 'Sprint #02 — Binary Heaps & Priority Queues', date: '12 Feb 2026', score: '86 / 100', percent: 86, rank: '#13 / 180' },
      { label: 'C3', title: 'Sprint #03 — AVL & Red-Black Tree Rotations', date: '15 Mar 2026', score: '88 / 100', percent: 88, rank: '#11 / 180' },
      { label: 'C4', title: 'Sprint #04 — BFS/DFS & Topological Ordering', date: '14 Apr 2026', score: '86 / 100', percent: 86, rank: '#12 / 180' }
    ]
  },
  CS202: {
    percent: 79,
    detail: 'Combinatorics & graph proof challenges',
    completed: 4,
    total: 4,
    bestRank: '#22 / 180',
    history: [
      { label: 'C1', title: 'Challenge #01 — Propositional Logic & Lattices', date: '24 Jan 2026', score: '75 / 100', percent: 75, rank: '#27 / 180' },
      { label: 'C2', title: 'Challenge #02 — Generating Functions & Recurrences', date: '20 Feb 2026', score: '78 / 100', percent: 78, rank: '#24 / 180' },
      { label: 'C3', title: 'Challenge #03 — Bipartite Matching & Hall Theorem', date: '22 Mar 2026', score: '82 / 100', percent: 82, rank: '#22 / 180' },
      { label: 'C4', title: 'Challenge #04 — Planar Graphs & Chromatic Bounds', date: '18 Apr 2026', score: '81 / 100', percent: 81, rank: '#22 / 180' }
    ]
  },
  CS101: {
    percent: 91,
    detail: 'C pointers, memory management & algorithmic foundations',
    completed: 4,
    total: 4,
    bestRank: '#06 / 180',
    history: [
      { label: 'C1', title: 'Contest #01 — Bitwise Operations & Control Flow', date: '12 Sep 2025', score: '88 / 100', percent: 88, rank: '#10 / 180' },
      { label: 'C2', title: 'Contest #02 — Arrays, Strings & Two Pointers', date: '10 Oct 2025', score: '90 / 100', percent: 90, rank: '#08 / 180' },
      { label: 'C3', title: 'Contest #03 — Pointers, Structs & Linked Lists', date: '07 Nov 2025', score: '92 / 100', percent: 92, rank: '#07 / 180' },
      { label: 'C4', title: 'Contest #04 — Custom Arena Memory Allocator', date: '28 Nov 2025', score: '94 / 100', percent: 94, rank: '#06 / 180' }
    ]
  }
};

export const recentContests: ContestRecord[] = [
  { id: 'c-101', title: 'Biweekly Contest #06 — Dynamic Programming & Memoization', subjectCode: 'E0460A', date: '17 Oct 2026', score: '340 / 400', percent: 85, rank: '#12 / 178', status: 'Completed' },
  { id: 'c-102', title: 'SQL Query Planner & Indexing Sprint #05', subjectCode: 'DB01B1', date: '14 Oct 2026', score: '92 / 100', percent: 92, rank: '#09 / 176', status: 'Reviewed' },
  { id: 'c-103', title: 'System Design Architecture Review — Consistent Hashing', subjectCode: 'SD01B1', date: '11 Oct 2026', score: '41 / 50', percent: 82, rank: '#14 / 180', status: 'Reviewed' },
  { id: 'c-104', title: 'ML Practicum #04 — Regularized Logistic Regression', subjectCode: 'X0240A', date: '08 Oct 2026', score: '81 / 100', percent: 81, rank: '#19 / 175', status: 'Reviewed' },
  { id: 'c-105', title: 'Frontend Engineering Challenge — Async Race Conditions', subjectCode: 'E0463A', date: '04 Oct 2026', score: '76 / 100', percent: 76, rank: '#24 / 179', status: 'Reviewed' }
];

export const demoWeek: WeekSchedule = {
  start: '2026-10-19',
  published: true,
  days: {
    '2026-10-19': [
      { from: '09:00', to: '11:00', code: 'E0465B', room: 'Room B1', topic: 'Deconstructing Complex Systems', type: 'Lecture' },
      { from: '11:15', to: '13:15', code: 'E0460A', room: 'Room B1', topic: 'Dynamic Programming & Memoization', type: 'Lecture' },
      { from: '14:15', to: '16:00', code: 'E0463A', room: 'Lab 204', topic: 'React Server Components & Auth', type: 'Lab' }
    ],
    '2026-10-20': [
      { from: '09:30', to: '11:30', code: 'SD01B1', room: 'Auditorium B', topic: 'Load Balancing & Consistent Hashing', type: 'Lecture' },
      { from: '11:45', to: '13:30', code: 'DB01B1', room: 'Room B1', topic: 'Query Optimization & Execution Plans', type: 'Lecture' },
      { from: '14:30', to: '16:30', code: 'X0240A', room: 'AI Lab 102', topic: 'Stochastic Gradient Descent Practicum', type: 'Lab' }
    ],
    '2026-10-21': [
      { from: '09:00', to: '11:00', code: 'E0460A', room: 'Room B1', topic: 'Knapsack & Subsequence DP Workshop', type: 'Workshop' },
      { from: '11:30', to: '13:15', code: 'E0463A', room: 'Room B1', topic: 'Session Cookies, JWTs & Middleware', type: 'Lecture' }
    ],
    '2026-10-22': [
      { from: '09:30', to: '11:30', code: 'DB01B1', room: 'Database Lab 201', topic: 'PostgreSQL EXPLAIN ANALYZE & Indexing', type: 'Lab' },
      { from: '12:00', to: '13:30', code: 'X0240A', room: 'Room B1', topic: 'L1/L2 Regularization & Cross-Validation', type: 'Lecture' },
      { from: '14:30', to: '16:00', code: 'E0465B', room: 'Seminar Hall', topic: 'Fermi Estimation Case Teardown', type: 'Workshop' }
    ],
    '2026-10-23': [
      { from: '10:00', to: '12:00', code: 'SD01B1', room: 'Room B1', topic: 'Distributed Caching & Singleflight', type: 'Lecture' },
      { from: '16:00', to: '17:30', code: 'SD01B1', room: 'Auditorium B', topic: 'Guest Lecture: Principal Architect', type: 'Workshop' }
    ],
    '2026-10-24': [
      { from: '10:00', to: '13:00', code: 'E0460A', room: 'Main Coding Arena', topic: 'Biweekly Algorithmic Contest #07', type: 'Lab' }
    ]
  }
};

export const feeRecords: FeeItem[] = [
  {
    id: 'fee-1',
    invoiceNo: 'INV-2026-S3-01',
    description: 'Tuition Fee',
    semester: 'Semester 3 2026',
    amount: 131250,
    dueDate: '05/05/2026',
    status: 'Overdue',
    receiptAvailable: false
  },
  {
    id: 'fee-2',
    invoiceNo: 'INV-2026-S3-02',
    description: 'Upskilling & Cloud Lab Fee',
    semester: 'Semester 3 2026',
    amount: 8750,
    dueDate: '05/05/2026',
    paidDate: '02/05/2026',
    status: 'Paid',
    receiptAvailable: true
  },
  {
    id: 'fee-3',
    invoiceNo: 'INV-2025-S2-02',
    description: 'Upskilling & Cloud Lab Fee',
    semester: 'Semester 2 2026',
    amount: 8750,
    dueDate: '05/12/2025',
    paidDate: '01/12/2025',
    status: 'Paid',
    receiptAvailable: true
  },
  {
    id: 'fee-4',
    invoiceNo: 'INV-2025-S2-01',
    description: 'Tuition Fee',
    semester: 'Semester 2 2026',
    amount: 131250,
    dueDate: '05/12/2025',
    paidDate: '01/12/2025',
    status: 'Paid',
    receiptAvailable: true
  },
  {
    id: 'fee-5',
    invoiceNo: 'INV-2025-S1-01',
    description: 'Composite Course & Admission Fee',
    semester: 'Semester 1 2025',
    amount: 140000,
    dueDate: '05/08/2025',
    paidDate: '29/07/2025',
    status: 'Paid',
    receiptAvailable: true
  }
];
